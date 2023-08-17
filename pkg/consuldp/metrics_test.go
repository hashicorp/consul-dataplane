// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/armon/go-metrics"
	"github.com/hashicorp/consul-dataplane/internal/bootstrap"
	metricscache "github.com/hashicorp/consul-dataplane/pkg/metrics-cache"
	"github.com/stretchr/testify/require"
)

var (
	dogStatsdAddr    = "127.0.0.1"
	envoyMetricsPort = 19000
	envoyMetricsAddr = "127.0.0.1"
	envoyMetricsUrl  = fmt.Sprintf("http://%s:%v/stats/prometheus", envoyMetricsAddr, envoyMetricsPort)

	emptyTags = []metrics.Label{}
)

func setupTestServerAndBuffer(t *testing.T) (*net.UDPConn, []byte) {
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%v:%v", dogStatsdAddr, 0))
	if err != nil {
		t.Fatal(err)
	}
	server, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatal(err)
	}
	return server, make([]byte, 1024)
}

func assertServerMatchesExpected(t *testing.T, server *net.UDPConn, buf []byte, expected string) {
	t.Helper()
	n, _ := server.Read(buf)
	msg := string(buf[:n])

	require.Equal(t, msg, expected, fmt.Sprintf("Line %s does not match expected: %s", msg, expected))

}

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

			// Mock get requests to Envoy and Proxy instance metrics
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

func (c *mockClient) Post(url string, contentType string, body io.Reader) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
	}, nil
}

func makeFakeMetric(url string) string {
	return fmt.Sprintf(`fake_metric{url="%s"} 1\n`, url)
}

func TestMetricsStatsD(t *testing.T) {
	tag := "my_tag"
	tagValue := "my_value"
	prefix := "consul_dataplane"

	testMetrics := []struct {
		Method   string
		Metric   []string
		Value    float32
		Tags     []metrics.Label
		Expected string
	}{
		{"SetGauge", []string{"foo", "bar"}, float32(42), emptyTags, fmt.Sprintf("%v.foo.bar:42.000000|g\n", prefix)},
		{"SetGauge", []string{"foo", "bar", "baz"}, float32(42), emptyTags, fmt.Sprintf("%v.foo.bar.baz:42.000000|g\n", prefix)},
		{"AddSample", []string{"sample", "thing"}, float32(4), emptyTags, fmt.Sprintf("%v.sample.thing:4.000000|ms\n", prefix)},
		{"IncrCounter", []string{"count", "me"}, float32(3), emptyTags, fmt.Sprintf("%v.count.me:3.000000|c\n", prefix)},

		{"SetGauge", []string{"foo", "baz"}, float32(42), []metrics.Label{{Name: tag, Value: ""}}, fmt.Sprintf("%v.foo.baz.:42.000000|g\n", prefix)},
		{"SetGauge", []string{"foo", "baz"}, float32(42), []metrics.Label{{Name: tag, Value: tagValue}}, fmt.Sprintf("%v.foo.baz.my_value:42.000000|g\n", prefix)},
		{"SetGauge", []string{"foo", "bar"}, float32(42), []metrics.Label{{Name: tag, Value: tagValue}, {Name: "other_tag", Value: "other_value"}}, fmt.Sprintf("%v.foo.bar.my_value.other_value:42.000000|g\n", prefix)},
	}

	server, buf := setupTestServerAndBuffer(t)
	defer server.Close()

	port := int(server.LocalAddr().(*net.UDPAddr).Port)

	telem := &TelemetryConfig{
		UseCentralConfig: true,
	}
	cacheSink := metricscache.NewSink()
	cfg := metrics.DefaultConfig(prefix)
	cfg.EnableHostname = false
	_, _ = metrics.NewGlobal(cfg, cacheSink)

	m := &metricsConfig{
		mu:                 sync.Mutex{},
		cfg:                telem,
		envoyAdminAddr:     envoyMetricsAddr,
		envoyAdminBindPort: envoyMetricsPort,
		errorExitCh:        make(chan struct{}),
		cacheSink:          cacheSink,

		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := m.startMetrics(ctx, &bootstrap.BootstrapConfig{StatsdURL: fmt.Sprintf("udp://%v:%v", dogStatsdAddr, port)})
	require.NoError(t, err)

	for _, tt := range testMetrics {
		t.Run(tt.Method, func(t *testing.T) {
			switch tt.Method {
			case "SetGauge":
				metrics.SetGaugeWithLabels(tt.Metric, tt.Value, tt.Tags)
			case "AddSample":
				metrics.AddSampleWithLabels(tt.Metric, tt.Value, tt.Tags)
			case "IncrCounter":
				metrics.IncrCounterWithLabels(tt.Metric, tt.Value, tt.Tags)
			}
			assertServerMatchesExpected(t, server, buf, tt.Expected)
		})
	}

}

func TestMetricsDatadogWithoutGlobalTags(t *testing.T) {
	tag := "my_tag"
	tagValue := "my_value"
	prefix := "consul_dataplane"

	testMetrics := []struct {
		Method   string
		Metric   []string
		Value    float32
		Tags     []metrics.Label
		Expected string
	}{
		{"SetGauge", []string{"foo", "bar"}, float32(42), emptyTags, fmt.Sprintf("%v.foo.bar:42|g", prefix)},
		{"SetGauge", []string{"foo", "bar", "baz"}, float32(42), emptyTags, fmt.Sprintf("%v.foo.bar.baz:42|g", prefix)},
		{"AddSample", []string{"sample", "thing"}, float32(4), emptyTags, fmt.Sprintf("%v.sample.thing:4.000000|ms", prefix)},
		{"IncrCounter", []string{"count", "me"}, float32(3), emptyTags, fmt.Sprintf("%v.count.me:3|c", prefix)},

		{"SetGauge", []string{"foo", "baz"}, float32(42), []metrics.Label{{Name: tag, Value: ""}}, fmt.Sprintf("%v.foo.baz:42|g|#my_tag", prefix)},
		{"SetGauge", []string{"foo", "baz"}, float32(42), []metrics.Label{{Name: tag, Value: tagValue}}, fmt.Sprintf("%v.foo.baz:42|g|#my_tag:my_value", prefix)},
		{"SetGauge", []string{"foo", "bar"}, float32(42), []metrics.Label{{Name: tag, Value: tagValue}, {Name: "other_tag", Value: "other_value"}}, fmt.Sprintf("%v.foo.bar:42|g|#my_tag:my_value,other_tag:other_value", prefix)},
	}

	server, buf := setupTestServerAndBuffer(t)
	defer server.Close()

	port := int(server.LocalAddr().(*net.UDPAddr).Port)

	telem := &TelemetryConfig{
		UseCentralConfig: true,
	}
	cacheSink := metricscache.NewSink()
	_, _ = metrics.NewGlobal(metrics.DefaultConfig(prefix), cacheSink)

	m := &metricsConfig{
		mu:                 sync.Mutex{},
		cfg:                telem,
		envoyAdminAddr:     envoyMetricsAddr,
		envoyAdminBindPort: envoyMetricsPort,
		errorExitCh:        make(chan struct{}),
		cacheSink:          cacheSink,

		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := m.startMetrics(ctx, &bootstrap.BootstrapConfig{DogstatsdURL: fmt.Sprintf("udp://%v:%v", dogStatsdAddr, port)})
	require.NoError(t, err)
	for _, tt := range testMetrics {
		t.Run(tt.Method, func(t *testing.T) {
			switch tt.Method {
			case "SetGauge":
				metrics.SetGaugeWithLabels(tt.Metric, tt.Value, tt.Tags)
			case "AddSample":
				metrics.AddSampleWithLabels(tt.Metric, tt.Value, tt.Tags)
			case "IncrCounter":
				metrics.IncrCounterWithLabels(tt.Metric, tt.Value, tt.Tags)
			}
			assertServerMatchesExpected(t, server, buf, tt.Expected)
		})
	}

}

func TestMetricsDatadogWithGlobalTags(t *testing.T) {
	tag := "my_tag"
	tagValue := "my_value"
	prefix := "consul_dataplane"
	globalTags := "gtag:gvalue"

	testMetrics := []struct {
		Method   string
		Metric   []string
		Value    float32
		Tags     []metrics.Label
		Expected string
	}{
		{"SetGauge", []string{"foo", "bar"}, float32(42), emptyTags, fmt.Sprintf("%v.foo.bar:42|g|#%v", prefix, globalTags)},
		{"SetGauge", []string{"foo", "bar", "baz"}, float32(42), emptyTags, fmt.Sprintf("%v.foo.bar.baz:42|g|#%v", prefix, globalTags)},
		{"AddSample", []string{"sample", "thing"}, float32(4), emptyTags, fmt.Sprintf("%v.sample.thing:4.000000|ms|#%v", prefix, globalTags)},
		{"IncrCounter", []string{"count", "me"}, float32(3), emptyTags, fmt.Sprintf("%v.count.me:3|c|#%v", prefix, globalTags)},

		{"SetGauge", []string{"foo", "baz"}, float32(42), []metrics.Label{{Name: tag, Value: ""}}, fmt.Sprintf("%v.foo.baz:42|g|#%v,my_tag", prefix, globalTags)},
		{"SetGauge", []string{"foo", "baz"}, float32(42), []metrics.Label{{Name: tag, Value: tagValue}}, fmt.Sprintf("%v.foo.baz:42|g|#%v,my_tag:my_value", prefix, globalTags)},
		{"SetGauge", []string{"foo", "bar"}, float32(42), []metrics.Label{{Name: tag, Value: tagValue}, {Name: "other_tag", Value: "other_value"}}, fmt.Sprintf("%v.foo.bar:42|g|#%v,my_tag:my_value,other_tag:other_value", prefix, globalTags)},
	}

	server, buf := setupTestServerAndBuffer(t)
	server.LocalAddr().Network()
	defer server.Close()
	port := int(server.LocalAddr().(*net.UDPAddr).Port)

	telem := &TelemetryConfig{
		UseCentralConfig: true,
	}
	cacheSink := metricscache.NewSink()
	_, _ = metrics.NewGlobal(metrics.DefaultConfig(prefix), cacheSink)

	m := &metricsConfig{
		mu:                 sync.Mutex{},
		cfg:                telem,
		envoyAdminAddr:     envoyMetricsAddr,
		envoyAdminBindPort: envoyMetricsPort,
		errorExitCh:        make(chan struct{}),
		cacheSink:          cacheSink,

		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := m.startMetrics(ctx, &bootstrap.BootstrapConfig{DogstatsdURL: fmt.Sprintf("udp://%v:%v", dogStatsdAddr, port), StatsTags: []string{globalTags}})
	require.NoError(t, err)
	for _, tt := range testMetrics {
		t.Run(tt.Method, func(t *testing.T) {
			switch tt.Method {
			case "SetGauge":
				metrics.SetGaugeWithLabels(tt.Metric, tt.Value, tt.Tags)
			case "AddSample":
				metrics.AddSampleWithLabels(tt.Metric, tt.Value, tt.Tags)
			case "IncrCounter":
				metrics.IncrCounterWithLabels(tt.Metric, tt.Value, tt.Tags)
			}
			assertServerMatchesExpected(t, server, buf, tt.Expected)
		})
	}
}

func TestParseAddr(t *testing.T) {
	testCases := map[string]struct {
		addr         string
		s            Stats
		expectedErr  error
		expectedAddr string
	}{
		"prometheus": {
			addr:        "udp://0.0.0.0:1234",
			s:           Prometheus,
			expectedErr: errors.New("prometheus not implemented"),
		},
		"statsd good": {
			addr:         "udp://0.0.0.0:1234",
			s:            Statsd,
			expectedAddr: "0.0.0.0:1234",
		},
		"statsd bad": {
			addr:        "unix://path/to/mysocket",
			s:           Statsd,
			expectedErr: errors.New("unsupported addr: unix://path/to/mysocket for sink type: statsD"),
		},
		"dogstatsd good udp": {
			addr:         "udp://0.0.0.0:1234",
			s:            Dogstatsd,
			expectedAddr: "0.0.0.0:1234",
		},
		"dogstatsd good unix": {
			addr:         "unix://path/to/mysocket",
			s:            Dogstatsd,
			expectedAddr: "unix://path/to/mysocket",
		},
		"dogstatsd bad tcp": {
			addr:        "tcp://0.0.0.0:1234",
			s:           Dogstatsd,
			expectedErr: errors.New("unsupported addr: tcp://0.0.0.0:1234 for sink type: dogstatsD"),
		},
		"dogstatsd HOST_IP": {
			addr:         "udp://${HOST_IP}:1234",
			s:            Dogstatsd,
			expectedAddr: "1.2.3.4:1234",
		},
		"dogstatsd bad env var": {
			addr:        "udp://${NOT_SUPPORTED}:1234",
			s:           Dogstatsd,
			expectedErr: errors.New("failed to parse address udp://${NOT_SUPPORTED}:1234"),
		},
	}

	t.Setenv("HOST_IP", "1.2.3.4")
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			addr, err := parseSinkAddr(tc.addr, tc.s)
			if tc.expectedErr != nil {
				require.Error(t, err)
				require.EqualValues(t, err, tc.expectedErr)
			} else {

				require.Equal(t, addr, tc.expectedAddr)
			}
		})
	}

}
