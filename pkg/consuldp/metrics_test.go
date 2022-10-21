package consuldp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/consul-dataplane/internal/bootstrap"
	metricscache "github.com/hashicorp/consul-dataplane/pkg/metrics-cache"
	"github.com/stretchr/testify/require"
)

var (
	envoyMetricsPort = 19000
	envoyMetricsAddr = "127.0.0.1"
	envoyMetricsUrl  = fmt.Sprintf("http://%s:%v/stats/prometheus", envoyMetricsAddr, envoyMetricsPort)
)

func TestMetricsServerClosed(t *testing.T) {
	telem := &TelemetryConfig{
		UseCentralConfig: true,
		Prometheus:       PrometheusTelemetryConfig{},
	}
	m := &metricsConfig{
		mu:                 sync.Mutex{},
		cfg:                telem,
		envoyAdminAddr:     envoyMetricsAddr,
		envoyAdminBindPort: envoyMetricsPort,
		errorExitCh:        make(chan struct{}),
		cacheSink:          metricscache.NewSink(),

		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	_ = m.startMetrics(ctx, &bootstrap.BootstrapConfig{PrometheusBindAddr: "nonempty"})
	require.Equal(t, m.running, true)
	cancel()
	require.Eventually(t, func() bool {
		return !m.running
	}, time.Second*2, time.Second)

}

func TestMetricsServerEnabled(t *testing.T) {
	mergedMetricsBackendBindAddr := mergedMetricsBackendBindHost + defaultMergedMetricsBackendBindPort
	cases := map[string]struct {
		telemetry  *TelemetryConfig
		bindAddr   string
		expMetrics []string
	}{
		"no service metrics": {
			telemetry: &TelemetryConfig{UseCentralConfig: true},
			bindAddr:  mergedMetricsBackendBindAddr,
			expMetrics: []string{
				makeFakeMetric(cdpMetricsUrl),
				makeFakeMetric(envoyMetricsUrl),
			},
		},
		"with service metrics": {
			telemetry: &TelemetryConfig{
				UseCentralConfig: true,
				Prometheus: PrometheusTelemetryConfig{
					ServiceMetricsURL: "fake-service-metrics-url",
				},
			},
			bindAddr: mergedMetricsBackendBindAddr,
			expMetrics: []string{
				makeFakeMetric(cdpMetricsUrl),
				makeFakeMetric(envoyMetricsUrl),
				makeFakeMetric("fake-service-metrics-url"),
			},
		},
		"custom scrape path": {
			telemetry: &TelemetryConfig{
				UseCentralConfig: true,
				Prometheus: PrometheusTelemetryConfig{
					ServiceMetricsURL: "fake-service-metrics-url",
					// ScrapePath does not affect Consul Dataplane's metrics server.
					// It only affects where Envoy serves metrics.
					ScrapePath: "/test/scrape/path",
				},
			},
			bindAddr: mergedMetricsBackendBindAddr,
			expMetrics: []string{
				makeFakeMetric(cdpMetricsUrl),
				makeFakeMetric(envoyMetricsUrl),
				makeFakeMetric("fake-service-metrics-url"),
			},
		},
		"custom merge port": {
			telemetry: &TelemetryConfig{
				UseCentralConfig: true,
				Prometheus: PrometheusTelemetryConfig{
					MergePort:         1234,
					ServiceMetricsURL: "fake-service-metrics-url",
					// ScrapePath does not affect Consul Dataplane's metrics server.
					// It only affects where Envoy serves metrics.
					ScrapePath: "/test/scrape/path",
				},
			},
			bindAddr: mergedMetricsBackendBindHost + "1234",
			expMetrics: []string{
				makeFakeMetric(cdpMetricsUrl),
				makeFakeMetric(envoyMetricsUrl),
				makeFakeMetric("fake-service-metrics-url"),
			},
		},
	}
	for name, c := range cases {
		c := c
		t.Run(name, func(t *testing.T) {

			m := &metricsConfig{
				mu:                 sync.Mutex{},
				cfg:                c.telemetry,
				envoyAdminAddr:     envoyMetricsAddr,
				envoyAdminBindPort: envoyMetricsPort,
				errorExitCh:        make(chan struct{}),
				cacheSink:          metricscache.NewSink(),

				client: &http.Client{
					Timeout: 10 * time.Second,
				},
			}

			require.NotNil(t, m)
			require.NotNil(t, m.client)
			require.NotNil(t, m.errorExitCh)
			require.IsType(t, &http.Client{}, m.client)
			require.Greater(t, m.client.(*http.Client).Timeout, time.Duration(0))

			// Mock get requests to Envoy and Service instance metrics
			// so that they return a fake metric string.
			m.client = &mockClient{}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := m.startMetrics(ctx, &bootstrap.BootstrapConfig{PrometheusBindAddr: "nonempty"})
			require.NoError(t, err)
			require.Equal(t, c.bindAddr, m.promScrapeServer.Addr)

			// Have consul-dataplane's metrics server start on an open port.
			// And figure out what port was used so we can make requests to it.
			// Conveniently, this seems to wait until the server is ready for requests.
			portCh := make(chan int, 1)
			m.promScrapeServer.Addr = "127.0.0.1:0"
			m.promScrapeServer.BaseContext = func(l net.Listener) context.Context {
				portCh <- l.Addr().(*net.TCPAddr).Port
				return context.Background()
			}

			var port int
			select {
			case port = <-portCh:
			case <-time.After(5 * time.Second):
			}

			require.NotEqual(t, port, 0, "test failed to figure out metrics server port")
			log.Printf("port = %v", port)

			url := fmt.Sprintf("http://127.0.0.1:%d/stats/prometheus", port)
			resp, err := http.Get(url)
			require.NoError(t, err)
			require.NotNil(t, resp)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			expMetrics := strings.Join(c.expMetrics, "")
			require.Equal(t, expMetrics, string(body))

		})
	}
}

type mockClient struct{}

func (c *mockClient) Get(url string) (*http.Response, error) {
	buf := bytes.NewBufferString(makeFakeMetric(url))
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(buf),
	}, nil
}

func makeFakeMetric(url string) string {
	return fmt.Sprintf(`fake_metric{url="%s"} 1\n`, url)
}
