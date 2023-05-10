// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	// "net/url"
	// "strconv"
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

// lifecycleConfig handles all configuration related to merging
// the metrics and presenting them on promScrapeServer
type lifecycleConfig struct {
	logger hclog.Logger

	envoyAdminAddr     string
	envoyAdminBindPort int

	// merged metrics config
	promScrapeServer *http.Server // the server that will serve all the merged metrics
	client           httpGetter   // the client that will scrape the urls
	urls             []string     // the urls that will be scraped

	// consuldp metrics server
	cdpLifecycleServer *http.Server // cdp metrics prometheus scrape server

	// lifecycle control
	errorExitCh chan struct{}
	running     bool
	mu          sync.Mutex
}

func (m *lifecycleConfig) startLifecycleServer(ctx context.Context, bcfg *bootstrap.BootstrapConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return nil
	}

	m.logger = hclog.FromContext(ctx).Named("metrics")
	m.running = true
	go func() {
		<-ctx.Done()
		m.stopLifecycleServer()
	}()

	// 2. Setup prometheus handler for the merged metrics endpoint that prometheus
	// will actually scrape.
	mux := http.NewServeMux()
	mux.HandleFunc("/stats/prometheus", m.mergedMetricsHandler)
	m.urls = []string{cdpLifecycleUrl, fmt.Sprintf("http://%s:%v/stats/prometheus", m.envoyAdminAddr, m.envoyAdminBindPort)}
	// if m.cfg != nil && m.cfg.Prometheus.ServiceMetricsURL != "" {
	// 	m.urls = append(m.urls, m.cfg.Prometheus.ServiceMetricsURL)
	// }

	// 3. Determine what the merged metrics bind port is. It can be set as a flag.
	mergedMetricsBackendBindPort := defaultMergedMetricsBackendBindPort
	// if m.cfg.Prometheus.MergePort != 0 {
	// 	mergedMetricsBackendBindPort = strconv.Itoa(m.cfg.Prometheus.MergePort)
	// }
	m.promScrapeServer = &http.Server{
		Addr:    mergedMetricsBackendBindHost + mergedMetricsBackendBindPort,
		Handler: mux,
	}
	// 4. Start prometheus metrics sink
	go m.startPrometheusMergedMetricsSink()

	return nil
}

// startPrometheusMergedMetricsSink starts the main merged metrics server that prometheus
// will actually be scraping.
func (m *lifecycleConfig) startPrometheusMergedMetricsSink() {
	m.logger.Info("starting merged metrics server", "address", m.promScrapeServer.Addr)
	err := m.promScrapeServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		m.logger.Error("failed to serve metrics requests", "error", err)
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

	if m.promScrapeServer != nil {
		m.logger.Info("stopping the merged  server")
		err := m.promScrapeServer.Close()
		if err != nil {
			m.logger.Warn("error while closing metrics server", "error", err)
			errs = multierror.Append(err, errs)
		}
	}
	if m.cdpLifecycleServer != nil {
		m.logger.Info("stopping consul dp promtheus server")
		err := m.cdpLifecycleServer.Close()
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

// mergedMetricsHandler responds with merged metrics from multiple sources:
// Consul Dataplane, Envoy and (optionally) the service/application. The Envoy
// and service metrics are scraped synchronously during the handling of this
// request.
func (m *lifecycleConfig) mergedMetricsHandler(rw http.ResponseWriter, _ *http.Request) {
	for _, url := range m.urls {
		m.logger.Debug("scraping url for merging", "url", url)
		if err := m.scrapeMetrics(rw, url); err != nil {
			m.scrapeError(rw, url, err)
			return
		}
	}
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

// runPrometheusCDPServer takes a prom.Gatherer that will create a handler
// for http calls to the metrics endpoint and return prometheus style metrics.
// Eventually these metrics will be scraped and merged.
func (m *lifecycleConfig) runPrometheusCDPServer(gather prom.Gatherer) {
	m.cdpLifecycleServer = &http.Server{
		Addr: cdpLifecycleBindAddr,
		Handler: promhttp.HandlerFor(gather, promhttp.HandlerOpts{
			ErrorHandling: promhttp.ContinueOnError,
		}),
	}
	err := m.cdpLifecycleServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		m.logger.Error("failed to serve metrics requests", "error", err)
		close(m.errorExitCh)
	}
}