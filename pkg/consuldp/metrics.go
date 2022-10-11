package consuldp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/consul-dataplane/internal/bootstrap"
	"github.com/hashicorp/go-hclog"
)

const (
	metricsBackendBindPort = "20100"
	metricsBackendBindAddr = "127.0.0.1:" + metricsBackendBindPort
)

type metricsConfig struct {
	logger      hclog.Logger
	cfg         *TelemetryConfig
	httpServer  *http.Server
	client      httpGetter
	errorExitCh chan struct{}
	running     bool

	mu       sync.Mutex
	cancelFn context.CancelFunc
}

func NewMetricsConfig(cfg *Config) *metricsConfig {
	return &metricsConfig{
		mu:                 sync.Mutex{},
		cfg:                cfg.Telemetry,
		errorExitCh:        make(chan struct{}),
		envoyAdminAddr:     cfg.Envoy.AdminBindAddress,
		envoyAdminBindPort: cfg.Envoy.AdminBindPort,

		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (m *metricsConfig) startMetrics(ctx context.Context, bcfg *bootstrap.BootstrapConfig) {
	if m.running {
		return
	}
	if m.cfg.UseCentralConfig {
		m.logger = hclog.FromContext(ctx).Named("metrics")
		ctx, cancel := context.WithCancel(ctx)
		m.mu.Lock()
		m.cancelFn = cancel
		m.running = true
		m.mu.Unlock()

		switch {
		case bcfg.PrometheusBindAddr != "":
			mux := http.NewServeMux()
			mux.HandleFunc("/stats/prometheus", m.mergedMetricsHandler)
			m.httpServer = &http.Server{
				Addr:    metricsBackendBindAddr,
				Handler: mux,
			}
			// Start prometheus metrics sink
			go m.startPrometheusMetricsSink(ctx)

		case bcfg.StatsdURL != "":
			// TODO: send merged metrics
		case bcfg.DogstatsdURL != "":
			// TODO: send merged metrics
		}
	}
}

func (m *metricsConfig) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancelFn != nil {
		m.cancelFn()
	}
}

func (m *metricsConfig) startPrometheusMetricsSink(ctx context.Context) {
	go func() {
		<-ctx.Done()
		m.stopPrometheusMetricSink()
	}()

	m.logger.Info("starting metrics server", "address", m.httpServer.Addr)
	err := m.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		m.logger.Error("failed to serve metrics requests", "error", err)
		close(m.errorExitCh)
	}
}

func (m *metricsConfig) stopPrometheusMetricSink() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	if m.httpServer != nil {
		m.logger.Info("stopping metrics server")
		err := m.httpServer.Close()
		if err != nil {
			m.logger.Warn("error while closing metrics server", "error", err)
			close(m.errorExitCh)
		}
	}
}

func (m *metricsConfig) metricsServerExited() <-chan struct{} {
	return m.errorExitCh
}

// mergedMetricsHandler responds with merged metrics from multiple sources:
// Consul Dataplane, Envoy and (optionally) the service/application. The Envoy
// and service metrics are scraped synchronously during the handling of this
// request.
func (m *metricsConfig) mergedMetricsHandler(rw http.ResponseWriter, _ *http.Request) {
	m.logger.Debug("scraping Envoy metrics", "url", envoyMetricsUrl)
	if err := m.scrapeMetrics(rw, envoyMetricsUrl); err != nil {
		m.scrapeError(rw, envoyMetricsUrl, err)
		return
	}

	if m.cfg == nil || m.cfg.Prometheus.ServiceMetricsURL == "" {
		return
	}
	url := m.cfg.Prometheus.ServiceMetricsURL
	m.logger.Debug("scraping service metrics", "url", url)
	if err := m.scrapeMetrics(rw, url); err != nil {
		m.scrapeError(rw, url, err)
		return
	}
}

// scrapeMetrics fetches metrics from the given url and copies them to the response.
func (m *metricsConfig) scrapeMetrics(rw http.ResponseWriter, url string) error {
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
func (m *metricsConfig) scrapeError(rw http.ResponseWriter, url string, err error) {
	m.logger.Error("failed to scrape metrics", "url", url, "error", err)
	msg := fmt.Sprintf("failed to scrape metrics at url %q", url)
	http.Error(rw, msg, http.StatusInternalServerError)
}

// non2xxCode returns true if code is not in the range of 200-299 inclusive.
func non2xxCode(code int) bool {
	return code < 200 || code >= 300
}
