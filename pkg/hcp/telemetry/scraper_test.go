// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package telemetry

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-uuid"
	prompb "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
)

func Test_scrape(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Read example response.
	body, err := os.ReadFile("testdata/envoy_admin_stats_prometheus")
	r.NoError(err)

	for name, tc := range map[string]struct {
		modScraper     func(*scraperImpl)
		responseWriter func(w http.ResponseWriter)
		expectedErr    error
	}{
		"success": {},
		"fail bad endpoint": {
			modScraper: func(s *scraperImpl) {
				s.envoyAdminHostPort = "foo"
			},
			expectedErr: errScraperFailedToIssueRequest,
		},
		"fail bad status code": {
			responseWriter: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectedErr: errScraperFailedStatusCode,
		},
		"fail invalid response": {
			responseWriter: func(w http.ResponseWriter) {
				fmt.Fprint(w, "foo")
			},
			expectedErr: errScraperFailedToParseResponse,
		},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			filters := []string{"a", "b"}

			// Create a test server.
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				// Validate request sort of.
				r.Equal(http.MethodGet, req.Method)
				values, err := url.ParseQuery(req.URL.RawQuery)
				r.NoError(err)
				r.Contains(values, "usedonly")
				r.Equal("a|b", values.Get("filter"))

				if tc.responseWriter != nil {
					tc.responseWriter(w)
					return
				}

				fmt.Fprint(w, string(body))
			}))
			defer ts.Close()

			// Create a scraper.
			s := newScraper(ts.Listener.Addr().String(), hclog.NewNullLogger())
			if tc.modScraper != nil {
				tc.modScraper(s.(*scraperImpl))
			}

			// Scrape.
			labels := pcommon.NewMap()
			labels.PutStr("foo", "bar")
			metrics, err := s.scrape(context.Background(), filters, labels)

			if tc.expectedErr != nil {
				r.Error(err)
				r.ErrorIs(err, tc.expectedErr)
				return
			}

			r.NoError(err)
			r.NotNil(metrics)
			r.NotEmpty(metrics.ResourceMetrics())

			req := pmetricotlp.NewExportRequestFromMetrics(metrics)
			r.Equal(req.Metrics().ResourceMetrics().Len(), 1)
			rMetrics := req.Metrics().ResourceMetrics().At(0)

			// Check resource attributes.
			r.Equal(rMetrics.Resource().Attributes().Len(), 1)
			r.Equal(rMetrics.Resource().Attributes().AsRaw()["foo"], "bar")

			scopeMetrics := rMetrics.ScopeMetrics()
			r.Equal(scopeMetrics.Len(), 1)

			subMetrics := scopeMetrics.At(0).Metrics()
			t.Log(subMetrics.Len())
			r.Greater(subMetrics.Len(), 1)

			// Check a few metrics.
			metricNames := map[string]struct{}{}
			for i := 0; i < subMetrics.Len(); i++ {
				metricNames[subMetrics.At(i).Name()] = struct{}{}
			}
			r.Contains(metricNames, "envoy_http_no_route")                 // counter
			r.Contains(metricNames, "envoy_server_initialization_time_ms") // histogram
			r.Contains(metricNames, "envoy_server_live")                   // gauge
		})
	}
}

/*
This is some benchmarking of the prometheus > otlp metrics conversion. The results are conservative since they include the
time taken to create the test metrics (UUIDs/values). But they indicate that the conversion process itself is inexpensive.

We expect ~100 metrics per scrape given metric filters from HCP that limit scraped Envoy metrics to those we use in the
HCP UI dashboards and topology graph. Results here indicate that a conversion of 100 metrics requires only ~28us.

go test -benchmem -run=^$ -tags integration -bench ^BenchmarkMetrics github.com/hashicorp/consul-dataplane/pkg/hcp/telemetry
goos: darwin
goarch: arm64
pkg: github.com/hashicorp/consul-dataplane/pkg/hcp/telemetry
BenchmarkMetrics10-10             472564              2396 ns/op            3144 B/op         91 allocs/op
BenchmarkMetrics100-10             53437             22658 ns/op           28699 B/op        814 allocs/op
BenchmarkMetrics1000-10             3980            261018 ns/op          289823 B/op       8027 allocs/op
BenchmarkMetrics10000-10             385           3105312 ns/op         3033627 B/op      81045 allocs/op
BenchmarkMetrics100000-10             10         100042417 ns/op        44813523 B/op    1192834 allocs/op
*/
func BenchmarkMetrics10(b *testing.B)     { benchmarkMetrics(b, 10) }
func BenchmarkMetrics100(b *testing.B)    { benchmarkMetrics(b, 100) }
func BenchmarkMetrics1000(b *testing.B)   { benchmarkMetrics(b, 1000) }
func BenchmarkMetrics10000(b *testing.B)  { benchmarkMetrics(b, 10000) }
func BenchmarkMetrics100000(b *testing.B) { benchmarkMetrics(b, 100000) }

func benchmarkMetrics(b *testing.B, count int) {
	metrics := newBenchmarkMetrics(b, count)

	for i := 0; i < b.N; i++ {
		convertMetrics(metrics, pcommon.NewMap(), time.Now())
	}
}

func newBenchmarkMetrics(b *testing.B, count int) map[string]*prompb.MetricFamily {
	b.Helper()

	metrics := map[string]*prompb.MetricFamily{}
	for i := 0; i < count; i++ {
		// Create a metric (without a type).
		name := testUUID(b)
		metrics[name] = &prompb.MetricFamily{
			Name: &[]string{name}[0],
			Metric: []*prompb.Metric{
				{
					Label: []*prompb.LabelPair{
						{
							Name:  &[]string{testUUID(b)}[0],
							Value: &[]string{testUUID(b)}[0],
						},
					},
				},
			},
		}

		// Create metric types.
		switch i % 4 {
		case 0:
			metrics[name].Metric[0].Counter = &prompb.Counter{
				Value: &[]float64{rand.Float64()}[0],
			}
		case 1:
			metrics[name].Metric[0].Gauge = &prompb.Gauge{
				Value: &[]float64{rand.Float64()}[0],
			}
		case 2:
			metrics[name].Metric[0].Histogram = &prompb.Histogram{
				SampleCount: &[]uint64{rand.Uint64()}[0],
				Bucket: []*prompb.Bucket{
					{
						UpperBound:      &[]float64{1}[0],
						CumulativeCount: &[]uint64{rand.Uint64()}[0],
					},
					{
						UpperBound:      &[]float64{5}[0],
						CumulativeCount: &[]uint64{rand.Uint64()}[0],
					},
					{
						UpperBound:      &[]float64{10}[0],
						CumulativeCount: &[]uint64{rand.Uint64()}[0],
					},
					{
						UpperBound:      &[]float64{20}[0],
						CumulativeCount: &[]uint64{rand.Uint64()}[0],
					},
					{
						UpperBound:      &[]float64{50}[0],
						CumulativeCount: &[]uint64{rand.Uint64()}[0],
					},
					{
						UpperBound:      &[]float64{100}[0],
						CumulativeCount: &[]uint64{rand.Uint64()}[0],
					},
					{
						UpperBound:      &[]float64{200}[0],
						CumulativeCount: &[]uint64{rand.Uint64()}[0],
					},
					{
						UpperBound:      &[]float64{500}[0],
						CumulativeCount: &[]uint64{rand.Uint64()}[0],
					},
					{
						UpperBound:      &[]float64{1000}[0],
						CumulativeCount: &[]uint64{rand.Uint64()}[0],
					},
				},
			}
		case 3:
			metrics[name].Metric[0].Summary = &prompb.Summary{
				SampleCount: &[]uint64{rand.Uint64()}[0],
				SampleSum:   &[]float64{rand.Float64()}[0],
				Quantile: []*prompb.Quantile{
					{
						Quantile: &[]float64{0.5}[0],
						Value:    &[]float64{rand.Float64()}[0],
					},
				},
			}
		}
	}

	return metrics
}

func testUUID(b *testing.B) string {
	b.Helper()

	name, err := uuid.GenerateUUID()
	require.NoError(b, err)
	return name
}
