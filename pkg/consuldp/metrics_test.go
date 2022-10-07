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
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestMetricsServerEnabled(t *testing.T) {
	cases := map[string]struct {
		telemetry  *TelemetryConfig
		expMetrics []string
	}{
		"no service metrics": {
			telemetry: &TelemetryConfig{UseCentralConfig: true},
			expMetrics: []string{
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
			expMetrics: []string{
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
			expMetrics: []string{
				makeFakeMetric(envoyMetricsUrl),
				makeFakeMetric("fake-service-metrics-url"),
			},
		},
	}
	for name, c := range cases {
		c := c
		t.Run(name, func(t *testing.T) {
			cdp := &ConsulDataplane{
				cfg:    &Config{Telemetry: c.telemetry},
				logger: hclog.NewNullLogger(),
			}
			cdp.setupMetricsServer()

			m := cdp.metricsServer
			require.NotNil(t, m)
			require.NotNil(t, m.httpServer)
			require.NotNil(t, m.client)
			require.NotNil(t, m.exitedCh)
			require.Equal(t, metricsBackendBindAddr, m.httpServer.Addr)
			require.IsType(t, &http.Client{}, m.client)
			require.Greater(t, m.client.(*http.Client).Timeout, time.Duration(0))

			// Mock get requests to Envoy and Service instance metrics
			// so that they return a fake metric string.
			m.client = &mockClient{}

			// Have consul-dataplane's metrics server start on an open port.
			// And figure out what port was used so we can make requests to it.
			// Conveniently, this seems to wait until the server is ready for requests.
			portCh := make(chan int, 1)
			m.httpServer.Addr = "127.0.0.1:0"
			m.httpServer.BaseContext = func(l net.Listener) context.Context {
				portCh <- l.Addr().(*net.TCPAddr).Port
				return context.Background()
			}

			go cdp.startMetricsServer()
			t.Cleanup(cdp.stopMetricsServer)

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
