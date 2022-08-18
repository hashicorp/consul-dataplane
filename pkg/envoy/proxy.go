package envoy

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/go-hclog"
)

type state uint16

const (
	stateInitial state = iota
	stateRunning
	stateStopped
)

// Proxy manages an Envoy proxy process.
//
// TODO(NET-118): properly handle the Envoy process lifecycle, including
// restarting crashed processes.
//
// Note: Proxy is not thread-safe, callers are responsible for synchronizing
// access to it.
type Proxy struct {
	cfg ProxyConfig

	state   state
	cmd     *exec.Cmd
	cleanup func() error
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
	Logger hclog.Logger

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
	return &Proxy{cfg: cfg}, nil
}

// Run the Envoy proxy process.
//
// The caller is responsible for terminating the Envoy process with Stop.
func (p *Proxy) Run() error {
	if p.state != stateInitial {
		return errors.New("proxy must not have already been run")
	}
	p.state = stateRunning

	// Write the bootstrap config to a pipe.
	var (
		configPath string
		err        error
	)
	configPath, p.cleanup, err = writeBootstrapConfig(p.cfg.BootstrapConfig)
	if err != nil {
		return err
	}

	// Run the Envoy process.
	p.cmd = p.buildCommand(configPath)
	p.cfg.Logger.Debug("running envoy proxy", "command", strings.Join(p.cmd.Args, " "))
	if err := p.cmd.Start(); err != nil {
		// Clean up the pipe if we weren't able to run Envoy.
		if err := p.cleanup(); err != nil {
			p.cfg.Logger.Error("failed to cleanup boostrap config", "error", err)
		}
		return err
	}
	return nil
}

// Stop the Envoy proxy process.
func (p *Proxy) Stop() error {
	if p.state != stateRunning {
		return errors.New("proxy must be running to be stopped")
	}
	p.state = stateStopped

	p.cfg.Logger.Debug("stopping envoy")

	err := p.cmd.Process.Kill()
	switch {
	case err == nil || errors.Is(err, os.ErrProcessDone):
		// Everything is fine!
	default:
		return err
	}

	// Reap the process so we don't leave a zombie.
	if _, err := p.cmd.Process.Wait(); err != nil {
		return err
	}

	return p.cleanup()
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
func (p *Proxy) buildCommand(cfgPath string) *exec.Cmd {
	args := append(
		// TODO: Do we want to enable/disable hot restart?
		[]string{"--config-path", cfgPath},
		p.cfg.ExtraArgs...,
	)

	cmd := exec.Command(p.cfg.ExecutablePath, args...)

	// TODO: send the logs somewhere more sensible.
	logger := p.cfg.Logger.Named("envoy").StandardWriter(&hclog.StandardLoggerOptions{})
	cmd.Stdout = logger
	cmd.Stderr = logger

	return cmd
}
