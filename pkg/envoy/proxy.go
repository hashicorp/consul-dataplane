// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package envoy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hashicorp/go-hclog"
)

type state uint32

const (
	stateInitial state = iota
	stateRunning
	stateStopped
)

const (
	logFormatPlain = "%Y-%m-%dT%T.%eZ%z [%l] envoy.%n(%t) %v"
	logFormatJSON  = `{"@timestamp":"%Y-%m-%dT%T.%fZ%z","@module":"envoy.%n","@level":"%l","@message":"%j","thread":%t}`
)

// Proxy manages an Envoy proxy process.
//
// TODO(NET-118): properly handle the Envoy process lifecycle, including
// restarting crashed processes.
type Proxy struct {
	cfg ProxyConfig

	state    state
	cmd      *exec.Cmd
	exitedCh chan struct{}
}

// ProxyConfig contains the configuration required to run an Envoy proxy.
type ProxyConfig struct {
	// ExecutablePath is the path to the Envoy executable.
	//
	// Defaults to whichever executable called envoy is found on $PATH.
	ExecutablePath string

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
		cfg:      cfg,
		exitedCh: make(chan struct{}),
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

	// Write the bootstrap config to a pipe.
	configPath, cleanup, err := writeBootstrapConfig(p.cfg.BootstrapConfig)
	if err != nil {
		return err
	}

	// Run the Envoy process.
	p.cmd = p.buildCommand(ctx, configPath)
	p.cfg.Logger.Debug("running envoy proxy", "command", strings.Join(p.cmd.Args, " "))
	if err := p.cmd.Start(); err != nil {
		// Clean up the pipe if we weren't able to run Envoy.
		if err := cleanup(); err != nil {
			p.cfg.Logger.Error("failed to cleanup boostrap config", "error", err)
		}
		return err
	}

	// This goroutine is responsible for waiting on the process (which reaps it
	// preventing a zombie), triggering cleanup, and notifying the caller that the
	// process has exited.
	go func() {
		err := p.cmd.Wait()
		p.cfg.Logger.Info("envoy process exited", "error", err)
		p.transitionState(stateRunning, stateStopped)
		if err := cleanup(); err != nil {
			p.cfg.Logger.Error("failed to cleanup boostrap config", "error", err)
		}
		close(p.exitedCh)
	}()

	return nil
}

// Stop the Envoy proxy process.
//
// Note: the caller is responsible for ensuring Stop is not called concurrently
// with Run, as this is thread-unsafe.
func (p *Proxy) Stop() error {
	switch p.getState() {
	case stateStopped:
		// Nothing to do!
		return nil
	case stateRunning:
		// Kill the process.
		p.cfg.Logger.Debug("stopping envoy")
		return p.cmd.Process.Kill()
	default:
		return errors.New("proxy must be running to be stopped")
	}
}

// Exited returns a channel that is closed when the Envoy process exits. It can
// be used to detect and act on process crashes.
func (p *Proxy) Exited() chan struct{} { return p.exitedCh }

func (p *Proxy) getState() state {
	return state(atomic.LoadUint32((*uint32)(&p.state)))
}

func (p *Proxy) transitionState(before, after state) bool {
	return atomic.CompareAndSwapUint32((*uint32)(&p.state), uint32(before), uint32(after))
}

// writeBootstrapConfig writes the given Envoy bootstrap config to a named pipe
// and returns the path. It also returns a cleanup function that must be called
// when Envoy is done with it.
//
// We use a named pipe rather than a tempfile because it prevents writing any
// secrets to disk. See: https://github.com/hashicorp/consul/pull/5964
func writeBootstrapConfig(cfg []byte) (string, func() error, error) {
	path := filepath.Join(
		os.TempDir(),
		fmt.Sprintf("envoy-%x-bootstrap.json", time.Now().UnixNano()+int64(os.Getpid())),
	)
	if err := syscall.Mkfifo(path, 0600); err != nil {
		return "", nil, err
	}

	// O_WRONLY causes OpenFile to block until there's a reader (Envoy). Opening
	// the pipe with O_RDWR wouldn't block but would result in just sending stuff
	// to ourself.
	//
	// TODO(boxofrad): We don't have a way to cancel this goroutine. If the Envoy
	// process never opens the other end of the pipe this will hang forever. The
	// workaround we use in `consul connect envoy` is to write to the pipe in a
	// subprocess that self-destructs after 10 minutes.
	go func() {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			os.Remove(path)
			return
		}

		_, err = file.Write(cfg)
		file.Close()

		if err != nil {
			os.Remove(path)
		}
	}()

	return path, func() error {
		err := os.Remove(path)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}, nil
}

// buildCommand builds the exec.Cmd to run Envoy with the relevant arguments
// (e.g. config path) and its logs redirected to the logger.
func (p *Proxy) buildCommand(ctx context.Context, cfgPath string) *exec.Cmd {
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

	args := append(
		[]string{
			"--config-path", cfgPath,
			"--log-format", logFormat,
			"--log-level", logLevel,

			// TODO(NET-713): support hot restarts.
			"--disable-hot-restart",
		},
		p.cfg.ExtraArgs...,
	)

	cmd := exec.CommandContext(ctx, p.cfg.ExecutablePath, args...)
	cmd.Stdout = p.cfg.EnvoyOutputStream
	cmd.Stderr = p.cfg.EnvoyErrorStream

	return cmd
}
