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
)

// lifecycleConfig handles all configuration related to managing the Envoy proxy
// lifecycle, including exposing management controls via an HTTP server.
type lifecycleConfig struct {
	logger hclog.Logger

	// consuldp proxy lifecycle management config
	shutdownDrainListeners bool
	shutdownGracePeriod    int
	gracefulPort           int
	gracefulShutdownPath   string

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
		shutdownDrainListeners: cfg.Envoy.ShutdownDrainListeners,
		shutdownGracePeriod:    cfg.Envoy.ShutdownGracePeriod,
		gracefulPort:           cfg.Envoy.GracefulPort,
		gracefulShutdownPath:   cfg.Envoy.GracefulShutdownPath,

		proxy: proxy,

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

	// Determine what HTTP endpoint paths to configure for the proxy lifecycle
	// management server bind port is. These can be set as flags.
	cdpLifecycleShutdownPath := defaultLifecycleShutdownPath
	if m.gracefulShutdownPath != "" {
		cdpLifecycleShutdownPath = m.gracefulShutdownPath
	}

	// Set config to allow introspection of default path for testing
	m.gracefulShutdownPath = cdpLifecycleShutdownPath

	m.logger.Info(fmt.Sprintf("setting graceful shutdown path: %s\n", cdpLifecycleShutdownPath))
	mux.HandleFunc(cdpLifecycleShutdownPath, m.gracefulShutdown)

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
		m.logger.Error("failed to serve proxy lifecycle managerments requests", "error", err)
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

// gracefulShutdown blocks until shutdownGracePeriod seconds have elapsed, and, if
// configured, will drain inbound connections to Envoy listeners during that time.
func (m *lifecycleConfig) gracefulShutdown(rw http.ResponseWriter, _ *http.Request) {
	m.logger.Info("initiating shutdown")

	// Create a context that  will signal a cancel at the specified duration.
	// TODO: should this use lifecycleManager ctx instead of context.Background?
	timeout := time.Duration(m.shutdownGracePeriod) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	m.logger.Info(fmt.Sprintf("waiting %d seconds before terminating dataplane proxy", m.shutdownGracePeriod))

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		// If shutdownDrainListeners enabled, initiatie graceful shutdown of Envoy.
		// We want to start draining connections from inbound listeners if
		// configured, but still allow outbound traffic until gracefulShutdownPeriod
		// has elapsed to facilitate a graceful application shutdown.
		if m.shutdownDrainListeners {
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

	// Return HTTP 200 Success
	rw.WriteHeader(http.StatusOK)
}
