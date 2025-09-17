// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package envoy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-hclog"
)

type state uint32

const (
	stateInitial state = iota
	stateRunning
	stateDraining
	stateStopped
	stateExited
)

const (
	logFormatPlain = "%Y-%m-%dT%T.%eZ%z [%l] envoy.%n(%t) %v"
	logFormatJSON  = `{"@timestamp":"%Y-%m-%dT%T.%fZ%z","@module":"envoy.%n","@level":"%l","@message":"%j","thread":%t}`
)

// ProxyManager is an interface for managing an Envoy proxy process.
type ProxyManager interface {
	Run(ctx context.Context) error
	Drain() error
	Quit() error
	Kill() error
	DumpConfig() error
	Ready() (bool, error)
}

// Proxy manages an Envoy proxy process.
//
// TODO(NET-118): properly handle the Envoy process lifecycle, including
// restarting crashed processes.
type Proxy struct {
	cfg ProxyConfig

	// client that will dial the managed Envoy proxy
	client *http.Client

	state    state
	cmd      *exec.Cmd
	exitedCh chan error
}

// ProxyConfig contains the configuration required to run an Envoy proxy.
type ProxyConfig struct {
	// ExecutablePath is the path to the Envoy executable.
	//
	// Defaults to whichever executable called envoy is found on $PATH.
	ExecutablePath string

	// AdminAddr is the hostname or IP address of the Envoy admin interface.
	//
	// Defaults to 127.0.0.1
	AdminAddr string

	// AdminBindPort is the port of the Envoy admin interface.
	//
	// Defaults to 19000
	AdminBindPort int

	// ExtraArgs are additional arguments that will be passed to Envoy.
	ExtraArgs []string

	// Logger that will be used to emit log messages.
	//
	// Note: Envoy logs are *not* written to this logger, and instead are written
	// directly to EnvoyOutputStream + EnvoyErrorStream.
	Logger hclog.Logger

	// LogJSON determines whether the logs emitted by Envoy will be in JSON format.
	LogJSON bool

	// EnvoyErrorStream is the io.Writer to which the Envoy output stream will be redirected.
	// Envoy writes process debug logs to the error stream.
	EnvoyErrorStream io.Writer

	// EnvoyOutputStream is the io.Writer to which the Envoy output stream will be redirected.
	// The default Consul access log configuration write logs to the output stream.
	EnvoyOutputStream io.Writer

	// BootstrapConfig is the Envoy bootstrap configuration (in YAML or JSON format)
	// that will be provided to Envoy via the --config-path flag.
	BootstrapConfig []byte
}

// NewProxy creates a Proxy with the given configuration.
//
// Use Run to start the Envoy proxy process.
func NewProxy(cfg ProxyConfig) (*Proxy, error) {
	if cfg.ExecutablePath == "" {
		var err error
		cfg.ExecutablePath, err = exec.LookPath("envoy")
		if err != nil {
			return nil, err
		}
	}
	if cfg.Logger == nil {
		cfg.Logger = hclog.NewNullLogger()
	}
	if len(cfg.BootstrapConfig) == 0 {
		return nil, errors.New("BootstrapConfig is required to run an Envoy proxy")
	}
	if cfg.EnvoyOutputStream == nil {
		cfg.EnvoyOutputStream = os.Stdout
	}
	if cfg.EnvoyErrorStream == nil {
		cfg.EnvoyErrorStream = os.Stderr
	}
	return &Proxy{
		cfg: cfg,

		client: &http.Client{
			Timeout: 10 * time.Second,
		},

		exitedCh: make(chan error),
	}, nil
}

// Run the Envoy proxy process.
//
// The caller is responsible for terminating the Envoy process with Stop. If it
// crashes the caller can be notified by receiving on the Exited channel.
//
// Run may only be called once. It is not possible to restart a stopped proxy.
func (p *Proxy) Run(ctx context.Context) error {
	if !p.transitionState(stateInitial, stateRunning) {
		return errors.New("proxy may only be run once")
	}

	// Run the Envoy process.
	p.cmd = p.buildCommand(ctx, string(p.cfg.BootstrapConfig))

	// Start Envoy in its own process group to avoid directly receiving
	// SIGTERM intended for consul-dataplane, let proxy manager handle
	// graceful shutdown if configured.
	p.cmd.SysProcAttr = getProcessAttr()

	p.cfg.Logger.Debug("running envoy proxy", "command", strings.Join(p.cmd.Args, " "))
	if err := p.cmd.Start(); err != nil {
		return err
	}

	// This goroutine is responsible for waiting on the process (which reaps it
	// preventing a zombie), triggering cleanup, and notifying the caller that the
	// process has exited.
	go func() {
		err := p.cmd.Wait()
		p.cfg.Logger.Info("envoy process exited", "error", err)
		p.transitionState(stateRunning, stateExited)
		p.exitedCh <- err
		close(p.exitedCh)
	}()

	return nil
}

// Start draining inbound connections to the Envoy proxy process.
//
// Note: the caller is responsible for ensuring Drain is not called concurrently
// with Run, as this is thread-unsafe.
func (p *Proxy) Drain() error {
	envoyDrainListenersUrl := fmt.Sprintf("http://%s:%v/drain_listeners?inboundonly&graceful", p.cfg.AdminAddr, p.cfg.AdminBindPort)
	switch p.getState() {
	case stateExited:
		// Nothing to do!
		return nil
	case stateStopped:
		// Nothing to do!
		return nil
	case stateDraining:
		// Nothing to do!
		return nil
	case stateRunning:
		// Start draining inbound connections.
		p.cfg.Logger.Debug("draining inbound connections to proxy")
		p.transitionState(stateRunning, stateDraining)
		_, err := p.client.Post(envoyDrainListenersUrl, "text/plain", nil)
		if err != nil {
			p.cfg.Logger.Error("envoy: failed to initiate listener drain", "error", err)
		}
		return err
	default:
		return errors.New("proxy must be running to drain connections")
	}
}

// Gracefully stop the Envoy proxy process.
//
// Note: the caller is responsible for ensuring Quit is not called concurrently
// with Run, as this is thread-unsafe.
func (p *Proxy) Quit() error {
	envoyShutdownUrl := fmt.Sprintf("http://%s/quitquitquit", net.JoinHostPort(p.cfg.AdminAddr, strconv.Itoa(p.cfg.AdminBindPort)))

	switch p.getState() {
	case stateExited, stateStopped:
		// Nothing to do!
		return nil
	case stateDraining:
		// Gracefully stop the process after draining connections.
		p.cfg.Logger.Debug("stopping proxy connection draining, starting graceful shutdown of Envoy proxy")
		p.transitionState(stateDraining, stateStopped)
		_, err := p.client.Post(envoyShutdownUrl, "text/plain", nil)
		if err != nil {
			p.cfg.Logger.Error("envoy: failed to quit", "error", err)
		}
		return err
	case stateRunning:
		// Gracefully stop the process.
		p.cfg.Logger.Debug("starting graceful shutdown of Envoy proxy")
		p.transitionState(stateRunning, stateStopped)
		_, err := p.client.Post(envoyShutdownUrl, "text/plain", nil)
		if err != nil {
			p.cfg.Logger.Error("envoy: failed to quit", "error", err)
		}
		return err
	default:
		return errors.New("proxy must be running to be stopped")
	}
}

// Forcefully kill the Envoy proxy process.
//
// Note: the caller is responsible for ensuring Stop is not called concurrently
// with Run, as this is thread-unsafe.
func (p *Proxy) Kill() error {
	switch p.getState() {
	case stateExited:
		// Nothing to do!
		return nil
	case stateStopped:
		// Kill the process, may have failed to gracefully stop.
		p.cfg.Logger.Debug("killing Envoy proxy process")
		return p.cmd.Process.Kill()
	case stateDraining:
		// Kill the process, may have failed to gracefully stop.
		p.cfg.Logger.Debug("killing Envoy proxy process")
		return p.cmd.Process.Kill()
	case stateRunning:
		// Kill the process.
		p.cfg.Logger.Debug("killing Envoy proxy process")
		return p.cmd.Process.Kill()
	default:
		return errors.New("proxy must be running to be killed")
	}
}

// Dump Envoy config to disk.
func (p *Proxy) DumpConfig() error {
	switch p.getState() {
	case stateExited:
		return errors.New("proxy must be running to dump config")
	case stateStopped:
		return errors.New("proxy must be running to dump config")
	case stateDraining:
		return p.dumpConfig()
	case stateRunning:
		return p.dumpConfig()
	default:
		return errors.New("proxy must be running to dump config")
	}
}

func (p *Proxy) dumpConfig() error {
	envoyConfigDumpUrl := fmt.Sprintf("http://%s:%v/config_dump?include_eds", p.cfg.AdminAddr, p.cfg.AdminBindPort)

	rsp, err := p.client.Get(envoyConfigDumpUrl)
	if err != nil {
		p.cfg.Logger.Error("envoy: failed to dump config", "error", err)
		return err
	}
	defer rsp.Body.Close()

	config, err := io.ReadAll(rsp.Body)
	if err != nil {
		p.cfg.Logger.Error("envoy: failed to dump config", "error", err)
		return err
	}

	if _, err := p.cfg.EnvoyOutputStream.Write(config); err != nil {
		p.cfg.Logger.Error("envoy: failed to write config to output stream", "error", err)
	}

	return err
}

// Exited returns a channel that is closed when the Envoy process exits. It can
// be used to detect and act on process crashes.
func (p *Proxy) Exited() chan error { return p.exitedCh }

func (p *Proxy) getState() state {
	return state(atomic.LoadUint32((*uint32)(&p.state)))
}

func (p *Proxy) transitionState(before, after state) bool {
	return atomic.CompareAndSwapUint32((*uint32)(&p.state), uint32(before), uint32(after))
}

// buildCommand builds the exec.Cmd to run Envoy with the relevant arguments
// (e.g. config path) and its logs redirected to the logger.
func (p *Proxy) buildCommand(ctx context.Context, cfgYaml string) *exec.Cmd {
	var logFormat string
	if p.cfg.LogJSON {
		logFormat = logFormatJSON
	} else {
		logFormat = logFormatPlain
	}

	// Infer the log level from the logger. We don't pass the config value as-is
	// because Envoy is slightly stricter about what it accepts than go-hclog.
	var (
		logger   = p.cfg.Logger
		logLevel string
	)
	switch {
	case logger.IsTrace():
		logLevel = "trace"
	case logger.IsDebug():
		logLevel = "debug"
	case logger.IsInfo():
		logLevel = "info"
	case logger.IsWarn():
		logLevel = "warn"
	case logger.IsError():
		logLevel = "error"
	default:
		logLevel = "info"
	}

	// Updating loglevel value if --log-level is present in extra args
	newExtraArgs, valOfLoggerInExtraArgs := removeArgAndGetValue(p.cfg.ExtraArgs, "--log-level")

	if len(valOfLoggerInExtraArgs) > 0 {
		logLevel = valOfLoggerInExtraArgs
	}

	args := append(
		[]string{
			"--config-yaml", cfgYaml,
			"--log-format", logFormat,
			"--log-level", logLevel,

			// TODO(NET-713): support hot restarts.
			"--disable-hot-restart",
		},
		newExtraArgs...,
	)

	cmd := exec.CommandContext(ctx, p.cfg.ExecutablePath, args...)
	cmd.Stdout = p.cfg.EnvoyOutputStream
	cmd.Stderr = p.cfg.EnvoyErrorStream

	return cmd
}

// removeArgAndGetValue Function to get new args after removing given key
// and also returns the value of key
func removeArgAndGetValue(stringAr []string, key string) ([]string, string) {
	for index, item := range stringAr {
		if item == key {
			valueToReturn := stringAr[index+1]
			return append(stringAr[:index], stringAr[index+2:]...), valueToReturn
		}
	}
	return stringAr, ""
}

func (p *Proxy) Ready() (bool, error) {

	switch p.getState() {
	case stateExited, stateStopped, stateDraining:
		// Nothing to do!
		return false, nil
	case stateRunning, stateInitial:
		// Query ready endpoint to check if proxy is Ready
		envoyReadyURL := fmt.Sprintf("http://%s:%v/ready", p.cfg.AdminAddr, p.cfg.AdminBindPort)
		rsp, err := p.client.Get(envoyReadyURL)
		if err != nil {
			p.cfg.Logger.Error("envoy: admin endpoint not available", "error", err)
			return false, err
		}
		if rsp != nil {
			defer rsp.Body.Close()
		}

		return rsp.StatusCode == 200, nil
	default:
		return false, nil
	}

}
