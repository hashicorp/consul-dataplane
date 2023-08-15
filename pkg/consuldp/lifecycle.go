// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-dataplane/pkg/envoy"
)

const (
	// defaultLifecycleBindPort is the port which will serve the proxy lifecycle HTTP
	// endpoints on the loopback interface.
	defaultLifecycleBindPort = "20300"
	cdpLifecycleBindAddr     = "127.0.0.1"
	cdpLifecycleUrl          = "http://" + cdpLifecycleBindAddr

	defaultLifecycleShutdownPath = "/graceful_shutdown"
	defaultLifecycleStartupPath  = "/graceful_startup"
)

// lifecycleConfig handles all configuration related to managing the Envoy proxy
// lifecycle, including exposing management controls via an HTTP server.
type lifecycleConfig struct {
	logger hclog.Logger

	// consuldp proxy lifecycle management config
	shutdownDrainListenersEnabled bool
	shutdownGracePeriodSeconds    int
	gracefulPort                  int
	gracefulShutdownPath          string
	startupGracePeriodSeconds     int
	gracefulStartupPath           string
	dumpEnvoyConfigOnExitEnabled  bool

	// manager for controlling the Envoy proxy process
	proxy envoy.ProxyManager

	// consuldp proxy lifecycle management server
	lifecycleServer *http.Server

	// consuldp proxy lifecycle server control
	errorExitCh chan struct{}
	running     bool
	mu          sync.Mutex
}

func NewLifecycleConfig(cfg *Config, proxy envoy.ProxyManager) *lifecycleConfig {
	return &lifecycleConfig{
		shutdownDrainListenersEnabled: cfg.Envoy.ShutdownDrainListenersEnabled,
		shutdownGracePeriodSeconds:    cfg.Envoy.ShutdownGracePeriodSeconds,
		gracefulPort:                  cfg.Envoy.GracefulPort,
		gracefulShutdownPath:          cfg.Envoy.GracefulShutdownPath,
		dumpEnvoyConfigOnExitEnabled:  cfg.Envoy.DumpEnvoyConfigOnExitEnabled,
		startupGracePeriodSeconds:     cfg.Envoy.StartupGracePeriodSeconds,
		gracefulStartupPath:           cfg.Envoy.GracefulStartupPath,
		proxy:                         proxy,

		errorExitCh: make(chan struct{}, 1),
		mu:          sync.Mutex{},
	}
}

func (m *lifecycleConfig) startLifecycleManager(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return nil
	}

	m.logger = hclog.FromContext(ctx).Named("lifecycle")
	m.running = true
	go func() {
		<-ctx.Done()
		m.stopLifecycleServer()
	}()

	// Start the server which will expose HTTP endpoints for proxy lifecycle
	// management control
	mux := http.NewServeMux()

	m.logger.Info(fmt.Sprintf("setting graceful shutdown path: %s\n", m.shutdownPath()))
	mux.HandleFunc(m.shutdownPath(), m.gracefulShutdownHandler)

	m.logger.Info(fmt.Sprintf("setting graceful startup path: %s\n", m.startupPath()))
	mux.HandleFunc(m.startupPath(), m.gracefulStartupHandler)

	// Determine what the proxy lifecycle management server bind port is. It can be
	// set as a flag.
	cdpLifecycleBindPort := defaultLifecycleBindPort
	if m.gracefulPort != 0 {
		cdpLifecycleBindPort = strconv.Itoa(m.gracefulPort)
	}
	m.lifecycleServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%s", cdpLifecycleBindAddr, cdpLifecycleBindPort),
		Handler: mux,
	}

	// Start the proxy lifecycle management server
	go m.startLifecycleServer()

	return nil
}

// startLifecycleServer starts the main proxy lifecycle management server that
// exposes HTTP endpoints for proxy lifecycle control.
func (m *lifecycleConfig) startLifecycleServer() {
	m.logger.Info("starting proxy lifecycle management server", "address", m.lifecycleServer.Addr)
	err := m.lifecycleServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		m.logger.Error("failed to serve proxy lifecycle management requests", "error", err)
		close(m.errorExitCh)
	}
}

// stopLifecycleServer stops the consul dataplane proxy lifecycle server
func (m *lifecycleConfig) stopLifecycleServer() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false

	if m.lifecycleServer != nil {
		m.logger.Info("stopping the lifecycle management server")
		err := m.lifecycleServer.Close()
		if err != nil {
			m.logger.Warn("error while closing lifecycle server", "error", err)
			close(m.errorExitCh)
		}
	}
}

// lifecycleServerExited is used to signal that the lifecycle server
// recieved a signal to initiate shutdown.
func (m *lifecycleConfig) lifecycleServerExited() <-chan struct{} {
	return m.errorExitCh
}

func (m *lifecycleConfig) gracefulShutdownHandler(rw http.ResponseWriter, _ *http.Request) {
	// Kick off graceful shutdown in a separate goroutine to avoid blocking
	// sending an HTTP response
	go m.gracefulShutdown()

	// Return HTTP 200 Success
	rw.WriteHeader(http.StatusOK)
}

// gracefulShutdown blocks until shutdownGracePeriod seconds have elapsed, and, if
// configured, will drain inbound connections to Envoy listeners during that time.
func (m *lifecycleConfig) gracefulShutdown() {
	m.logger.Info("initiating shutdown")

	// Create a context that  will signal a cancel at the specified duration.
	// TODO: should this use lifecycleManager ctx instead of context.Background?
	timeout := time.Duration(m.shutdownGracePeriodSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if m.dumpEnvoyConfigOnExitEnabled {
		m.logger.Info("dumping Envoy config to disk")
		err := m.proxy.DumpConfig()
		if err != nil {
			m.logger.Warn("error while attempting to dump Envoy config to disk", "error", err)
			close(m.errorExitCh)
		}
	}

	m.logger.Info(fmt.Sprintf("waiting %d seconds before terminating dataplane proxy", m.shutdownGracePeriodSeconds))

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		// If shutdownDrainListenersEnabled, initiatie graceful shutdown of Envoy.
		// We want to start draining connections from inbound listeners if
		// configured, but still allow outbound traffic until gracefulShutdownPeriod
		// has elapsed to facilitate a graceful application shutdown.
		if m.shutdownDrainListenersEnabled {
			err := m.proxy.Drain()
			if err != nil {
				m.logger.Warn("error while draining Envoy listeners", "error", err)
				close(m.errorExitCh)
			}
		}

		// Block until context timeout has elapsed
		<-ctx.Done()

		// Finish graceful shutdown, quit Envoy proxy
		m.logger.Info("shutdown grace period timeout reached")
		err := m.proxy.Quit()
		if err != nil {
			m.logger.Warn("error while shutting down Envoy", "error", err)
			close(m.errorExitCh)
		}
	}()

	// Wait for context timeout to elapse
	wg.Wait()
}

func (m *lifecycleConfig) gracefulStartupHandler(rw http.ResponseWriter, _ *http.Request) {

	//Unlike in gracefulShutdown, we want to delay the OK response until envoy is ready
	//in order to block application container.
	m.gracefulStartup()
	rw.WriteHeader(http.StatusOK)

}

// gracefulStartup blocks until the startup grace period has elapsed or we have confirmed that
// Envoy proxy is ready.
func (m *lifecycleConfig) gracefulStartup() {
	timeout := time.Duration(m.startupGracePeriodSeconds) * time.Second
	m.logger.Info(fmt.Sprintf("blocking container startup until Envoy ready or grace period of %d seconds elapsed", m.startupGracePeriodSeconds))
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	envoyReady := false
	var wg sync.WaitGroup
	wg.Add(1)
	envoyStatus := make(chan bool)

	go func() {
		defer wg.Done()
		envoyReady, _ = m.proxy.Ready()
		envoyStatus <- envoyReady

		//Loop until either proxy is ready or timeout expires.
	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			case envoyReady = <-envoyStatus:
				if envoyReady {
					break loop
				} else {
					//Check if Envoy is ready, don't block here so that timeout break can still happen.
					go func() {
						ready, _ := m.proxy.Ready()
						envoyStatus <- ready
					}()
				}

			}
		}

	}()

	wg.Wait()

	if !envoyReady {
		m.logger.Info("startup grace period reached before envoy ready")
	}

}

func (m *lifecycleConfig) shutdownPath() string {
	if m.gracefulShutdownPath == "" {
		// Set config to allow introspection of default path for testing
		m.gracefulShutdownPath = defaultLifecycleShutdownPath
	}

	return m.gracefulShutdownPath
}

func (m *lifecycleConfig) startupPath() string {
	if m.gracefulStartupPath == "" {
		// Set config to allow introspection of default path for testing
		m.gracefulStartupPath = defaultLifecycleStartupPath
	}

	return m.gracefulStartupPath
}
