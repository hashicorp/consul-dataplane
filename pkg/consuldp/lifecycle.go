// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	// "net/url"
	"strconv"
	"sync"
	// "time"

	// "github.com/hashicorp/consul-server-connection-manager/discovery"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"

	"github.com/hashicorp/consul-dataplane/internal/bootstrap"
)

const (
	// defaultLifecycleBindPort is the port which will serve the proxy lifecycle HTTP
	// endpoints on the loopback interface.
	defaultLifecycleBindPort = "20300"
	cdpLifecycleBindAddr     = "127.0.0.1:" + defaultLifecycleBindPort
	cdpLifecycleUrl          = "http://" + cdpLifecycleBindAddr
)

// lifecycleConfig handles all configuration related to managing the Envoy proxy
// lifecycle, including exposing management controls via an HTTP server.
type lifecycleConfig struct {
	logger hclog.Logger

	envoyAdminAddr     string
	envoyAdminBindPort int

	// consuldp proxy lifecycle management config
	gracefulPort         int
	gracefulShutdownPath string
	client               httpGetter // client that will dial the managed Envoy proxy

	// consuldp proxy lifecycle management server
	lifecycleServer *http.Server

	// consuldp proxy lifecycle server control
	errorExitCh chan struct{}
	running     bool
	mu          sync.Mutex
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
	mux.HandleFunc(m.gracefulShutdownPath, m.gracefulShutdown)

	// Determine what the proxy lifecycle management server bind port is. It can be
	// set as a flag.
	cdpLifecycleBindPort := defaultLifecycleBindPort
	if m.gracefulPort != 0 {
		cdpLifecycleBindPort = strconv.Itoa(m.gracefulPort)
	}
	m.lifecycleServer = &http.Server{
		Addr:    cdpLifecycleBindAddr + cdpLifecycleBindPort,
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

// stopLifecycleServer stops the main merged metrics server and the consul
// dataplane metrics server
func (m *lifecycleConfig) stopLifecycleServer() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	var errs error

	if m.lifecycleServer != nil {
		m.logger.Info("stopping the merged  server")
		err := m.lifecycleServer.Close()
		if err != nil {
			m.logger.Warn("error while closing metrics server", "error", err)
			errs = multierror.Append(err, errs)
		}
	}
	if m.lifecycleServer != nil {
		m.logger.Info("stopping consul dp promtheus server")
		err := m.lifecycleServer.Close()
		if err != nil {
			m.logger.Warn("error while closing metrics server", "error", err)
			errs = multierror.Append(err, errs)
		}
	}
	// Check if there were errors and then close the error channel
	if errs != nil {
		close(m.errorExitCh)
	}
}

// lifecycleServerExited is used to signal that the metrics server
// exited unexpectedely.
func (m *lifecycleConfig) lifecycleServerExited() <-chan struct{} {
	return m.errorExitCh
}

// gracefulShutdown blocks until at most shutdownGracePeriod seconds have elapsed,
// or, if configured, until all open connections to Envoy listeners have been
// drained.
func (m *lifecycleConfig) gracefulShutdown(rw http.ResponseWriter, _ *http.Request) {
	envoyShutdownUrl := fmt.Sprintf("http://%s:%v/quitquitquit", m.envoyAdminAddr, m.envoyAdminBindPort)

	m.logger.Debug("initiating graceful shutdown")
	m.logger.Debug("shutting down Envoy", envoyShutdownUrl)

	// TODO: implement
}

// mergedMetricsHandler responds with merged metrics from multiple sources:
// Consul Dataplane, Envoy and (optionally) the service/application. The Envoy
// and service metrics are scraped synchronously during the handling of this
// request.
func (m *lifecycleConfig) mergedMetricsHandler(rw http.ResponseWriter, _ *http.Request) {
	// if err := m.scrapeMetrics(rw, url); err != nil {
	//     m.scrapeError(rw, url, err)
	//     return
	// }
}

// scrapeMetrics fetches metrics from the given url and copies them to the response.
func (m *lifecycleConfig) scrapeMetrics(rw http.ResponseWriter, url string) error {
	resp, err := m.client.Get(url)
	if err != nil {
		return err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			m.logger.Warn("failed to close metrics request", "error", err)
		}
	}()

	if non2xxCode(resp.StatusCode) {
		return fmt.Errorf("status code %d", resp.StatusCode)
	}

	// Prometheus metrics are joined by newlines, so when merging metrics
	// metrics we simply write all lines from each source to the response.
	_, err = io.Copy(rw, resp.Body)
	return err
}

// scrapeError logs an error and responds to the http request with an error.
func (m *lifecycleConfig) scrapeError(rw http.ResponseWriter, url string, err error) {
	m.logger.Error("failed to scrape metrics", "url", url, "error", err)
	msg := fmt.Sprintf("failed to scrape metrics at url %q", url)
	http.Error(rw, msg, http.StatusInternalServerError)
}
