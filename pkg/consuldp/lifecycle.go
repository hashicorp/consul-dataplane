// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"context"
	"fmt"
	"net/http"
	// "net/url"
	"strconv"
	"sync"
	"time"

	// "github.com/hashicorp/consul-server-connection-manager/discovery"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"

	"github.com/hashicorp/consul-dataplane/internal/bootstrap"
)

const (
	// defaultLifecycleBindPort is the port which will serve the proxy lifecycle HTTP
	// endpoints on the loopback interface.
	defaultLifecycleBindPort = "20300"
	cdpLifecycleBindAddr     = "127.0.0.1"
	cdpLifecycleUrl          = "http://" + cdpLifecycleBindAddr
)

// lifecycleConfig handles all configuration related to managing the Envoy proxy
// lifecycle, including exposing management controls via an HTTP server.
type lifecycleConfig struct {
	logger hclog.Logger

	envoyAdminAddr     string
	envoyAdminBindPort int
	envoyDrainTime     int

	// consuldp proxy lifecycle management config
	shutdownDrainListeners bool
	shutdownGracePeriod    int
	gracefulPort           int
	gracefulShutdownPath   string

	// client that will dial the managed Envoy proxy
	client httpClient

	// consuldp proxy lifecycle management server
	lifecycleServer *http.Server

	// consuldp proxy lifecycle server control
	errorExitCh chan struct{}
	running     bool
	mu          sync.Mutex
}

func NewLifecycleConfig(cfg *Config) *lifecycleConfig {
	return &lifecycleConfig{
		envoyAdminAddr:     cfg.Envoy.AdminBindAddress,
		envoyAdminBindPort: cfg.Envoy.AdminBindPort,
		envoyDrainTime:     cfg.Envoy.EnvoyDrainTime,

		shutdownDrainListeners: cfg.Envoy.ShutdownDrainListeners,
		shutdownGracePeriod:    cfg.Envoy.ShutdownGracePeriod,
		gracefulPort:           cfg.Envoy.GracefulPort,
		gracefulShutdownPath:   cfg.Envoy.GracefulShutdownPath,

		client: &http.Client{
			Timeout: 10 * time.Second,
		},

		errorExitCh: make(chan struct{}),
		mu:          sync.Mutex{},
	}
}

func (m *lifecycleConfig) startLifecycleManager(ctx context.Context, bcfg *bootstrap.BootstrapConfig) error {
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
	fmt.Printf("graceful shutdown path: %s\n", m.gracefulShutdownPath)
	// TODO: set a default value in lifecycle manager init instead of empty string
	// to avoid panic here
	m.gracefulShutdownPath = "/shutdown"

	mux.HandleFunc(m.gracefulShutdownPath, m.gracefulShutdown)

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
	var errs error

	if m.lifecycleServer != nil {
		m.logger.Info("stopping the lifecycle management server")
		err := m.lifecycleServer.Close()
		if err != nil {
			m.logger.Warn("error while closing lifecycle server", "error", err)
			errs = multierror.Append(err, errs)
		}
	}

	// Check if there were errors and then close the error channel
	if errs != nil {
		close(m.errorExitCh)
	}
}

// lifecycleServerExited is used to signal that the lifecycle server
// recieved a signal to initiate shutdown.
// func (m *lifecycleConfig) lifecycleServerExited() <-chan struct{} {
// 	return m.errorExitCh
// }

// gracefulShutdown blocks until at most shutdownGracePeriod seconds have elapsed,
// or, if configured, until all open connections to Envoy listeners have been
// drained.
func (m *lifecycleConfig) gracefulShutdown(rw http.ResponseWriter, _ *http.Request) {
	envoyDrainListenersUrl := fmt.Sprintf("http://%s:%v/drain_listeners?inboundonly&graceful", m.envoyAdminAddr, m.envoyAdminBindPort)
	envoyShutdownUrl := fmt.Sprintf("http://%s:%v/quitquitquit", m.envoyAdminAddr, m.envoyAdminBindPort)

	m.logger.Info("initiating shutdown")

	// Wait until shutdownGracePeriod seconds have elapsed before actually
	// terminating the Envoy proxy process.
	m.logger.Info(fmt.Sprintf("waiting %d seconds before terminating dataplane proxy", m.shutdownGracePeriod))
	timeout := time.Duration(m.shutdownGracePeriod) * time.Second

	// Create a context that is both manually cancellable and will signal
	// a cancel at the specified duration.
	// TODO: should this use lifecycleManager ctx instead of context.Background?
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	go func() {
		// If shutdownDrainListeners enabled, initiatie graceful shutdown of Envoy.
		// We want to start draining connections from inbound listeners if
		// configured, but still allow outbound traffic until gracefulShutdownPeriod
		// has elapsed to facilitate a graceful application shutdown.
		if m.shutdownDrainListeners {
			_, err := m.client.Post(envoyDrainListenersUrl, "text/plain", nil)
			if err != nil {
				m.logger.Error("envoy: failed to initiate listener drain", "error", err)
				close(m.errorExitCh)
			}
		}

		for {
			select {
			case <-ctx.Done():
				m.logger.Info("shutdown grace period timeout reached")
				_, err := m.client.Post(envoyShutdownUrl, "text/plain", nil)
				if err != nil {
					m.logger.Error("envoy: failed to initiate listener drain", "error", err)
					close(m.errorExitCh)
				}
			}
			// TODO: is there a need to handle context cancelation here if not
			// able to shutdown cleanly?
		}

		// TODO: is there actually any point to sending a signal if we always just
		// want to wait unitl the shutdownGracePeriod has elapsed?
	}()

	// Return HTTP 200 Success
	rw.WriteHeader(http.StatusOK)
}
