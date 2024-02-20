// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package telemetry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
	prompb "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var (
	errScraperFailedToCreateRequest = errors.New("failed to create request")
	errScraperFailedToIssueRequest  = errors.New("failed to issue request")
	errScraperFailedStatusCode      = errors.New("non-200 status code")
	errScraperFailedToParseResponse = errors.New("failed to parse response")
)

type scraper interface {
	scrape(ctx context.Context, filters []string, labels pcommon.Map) (pmetric.Metrics, error)
}

type scraperImpl struct {
	client             http.RoundTripper
	envoyAdminHostPort string
	logger             hclog.Logger
	parser             *expfmt.TextParser
}

func newScraper(envoyAdminHostPort string, logger hclog.Logger) scraper {
	return &scraperImpl{
		client:             http.DefaultTransport,
		envoyAdminHostPort: envoyAdminHostPort,
		logger:             logger,
		parser:             new(expfmt.TextParser),
	}
}

// scrape is called by the exporter goroutine to scrape metrics from the Envoy admin API.
//
// This calls Envoy's /stats/prometheus endpoint with filter set to OR'ed combination of regexp filters.
// https://www.envoyproxy.io/docs/envoy/latest/operations/admin#get--stats-prometheus
func (s *scraperImpl) scrape(ctx context.Context, filters []string, labels pcommon.Map) (pmetric.Metrics, error) {
	// Actually call scrape against the Envoy admin API.
	scrapeTime := time.Now() // timestamp that's used for all metrics w/o timestamps (which is most of them).
	promMetrics, err := s.runScrape(ctx, filters)
	if err != nil {
		return pmetric.Metrics{}, fmt.Errorf("failed to scrape metrics: %w", err)
	}

	return convertMetrics(promMetrics, labels, scrapeTime), nil
}

func (s *scraperImpl) runScrape(ctx context.Context, filters []string) (map[string]*prompb.MetricFamily, error) {
	filterCombined := strings.Join(filters, "|") // just OR'ing all the filters together.

	params := &url.Values{}
	// envoy docs: Filters the returned stats to those with names matching the regular expression regex.
	// Compatible with usedonly. Performs partial matching by default, so /stats?filter=server will return
	// all stats containing the word server. Full-string matching can be specified with begin- and end-line anchors.
	// (i.e. /stats?filter=^server.concurrency$)
	params.Add("filter", filterCombined)
	// envoy docs: You can optionally pass the usedonly URL query parameter to only get statistics
	// that Envoy has updated (counters incremented at least once, gauges changed at least once,
	// and histograms added to at least once).
	params.Add("usedonly", "")
	url := fmt.Sprintf("http://%s/stats/prometheus?%s", s.envoyAdminHostPort, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errScraperFailedToCreateRequest, err)
	}

	resp, err := s.client.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errScraperFailedToIssueRequest, err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			s.logger.Warn("failed to close metrics request", "error", err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: %d", errScraperFailedStatusCode, resp.StatusCode)
	}

	metricFamilies, err := s.parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errScraperFailedToParseResponse, err)
	}

	return metricFamilies, nil
}

// convertMetrics takes in a map of prometheus metrics and converts them to OTLP metrics.
func convertMetrics(promMetrics map[string]*prompb.MetricFamily, labels pcommon.Map, scrapeTime time.Time) pmetric.Metrics {
	otlpMetrics := pmetric.NewMetrics()
	otlpResourceMetrics := otlpMetrics.ResourceMetrics().AppendEmpty()
	labels.CopyTo(otlpResourceMetrics.Resource().Attributes())

	otlpScopeMetrics := otlpResourceMetrics.ScopeMetrics().AppendEmpty()
	for _, family := range promMetrics {
		metric := otlpScopeMetrics.Metrics().AppendEmpty()
		metric.SetName(normalizeName(family.GetName()))
		metric.SetDescription(family.GetHelp())
		switch family.GetType() {
		case prompb.MetricType_COUNTER:
			setCounter(family, metric, scrapeTime)
		case prompb.MetricType_GAUGE:
			setGauge(family, metric, scrapeTime)
		case prompb.MetricType_HISTOGRAM:
			setHistogram(family, metric, scrapeTime)
		}
	}

	return otlpMetrics
}

func setCounter(family *prompb.MetricFamily, otlpMetric pmetric.Metric, scrapeTime time.Time) {
	emptySum := otlpMetric.SetEmptySum()
	emptySum.SetIsMonotonic(true)
	emptySum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	for _, metric := range family.GetMetric() {
		dp := emptySum.DataPoints().AppendEmpty()

		for _, labelPair := range metric.GetLabel() {
			dp.Attributes().PutStr(labelPair.GetName(), labelPair.GetValue())
		}

		dp.SetTimestamp(timestampFromMs(metric.GetTimestampMs(), scrapeTime))
		dp.SetDoubleValue(metric.GetCounter().GetValue())
	}
}

func setGauge(family *prompb.MetricFamily, otlpMetric pmetric.Metric, scrapeTime time.Time) {
	emptyGauge := otlpMetric.SetEmptyGauge()
	for _, metric := range family.GetMetric() {
		dp := emptyGauge.DataPoints().AppendEmpty()

		for _, labelPair := range metric.GetLabel() {
			dp.Attributes().PutStr(labelPair.GetName(), labelPair.GetValue())
		}

		dp.SetTimestamp(timestampFromMs(metric.GetTimestampMs(), scrapeTime))
		dp.SetDoubleValue(metric.GetGauge().GetValue())
	}
}

func setHistogram(family *prompb.MetricFamily, otlpMetric pmetric.Metric, scrapeTime time.Time) {
	emptyHistogram := otlpMetric.SetEmptyHistogram()
	emptyHistogram.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	for _, metric := range family.GetMetric() {
		histogram := metric.GetHistogram()

		if !isValidHistogram(histogram) {
			continue
		}

		dp := emptyHistogram.DataPoints().AppendEmpty()
		dp.SetCount(histogram.GetSampleCount())
		dp.SetSum(histogram.GetSampleSum())

		bounds, bucket := getBoundsAndBuckets(histogram)

		dp.BucketCounts().FromRaw(bucket)
		dp.ExplicitBounds().FromRaw(bounds)

		for _, labelPair := range metric.GetLabel() {
			dp.Attributes().PutStr(labelPair.GetName(), labelPair.GetValue())
		}

		dp.SetTimestamp(timestampFromMs(metric.GetTimestampMs(), scrapeTime))
	}
}

func getBoundsAndBuckets(histogram *prompb.Histogram) (bounds []float64, bucketCount []uint64) {
	bounds = []float64{}
	bucketCount = []uint64{}

	for _, bucket := range histogram.GetBucket() {
		if math.IsNaN(bucket.GetUpperBound()) {
			continue
		}
		bounds = append(bounds, bucket.GetUpperBound())
		bucketCount = append(bucketCount, bucket.GetCumulativeCount())
	}

	return bounds, bucketCount
}

func isValidHistogram(histogram *prompb.Histogram) bool {
	if histogram.SampleCount == nil || histogram.SampleSum == nil {
		return false
	}

	if len(histogram.GetBucket()) == 0 {
		return false
	}
	return true
}

const (
	suffixCount   = "_count"
	suffixBucket  = "_bucket"
	suffixSum     = "_sum"
	suffixTotal   = "_total"
	suffixInfo    = "_info"
	suffixCreated = "_created"
)

var (
	suffixes = []string{suffixCreated, suffixBucket, suffixInfo, suffixSum, suffixCount}
)

// timestampFromMs converts a timestamp in milliseconds to a Timestamp.
//
// By default, Envoy's Prometheus metrics do _not_ include a timestamp which is an optional column.
// Because it's missing, we just use the timestamp of the metrics scrape instead and use that
// https://github.com/prometheus/docs/blob/main/content/docs/instrumenting/exposition_formats.md#comments-help-text-and-type-information
func timestampFromMs(timestampMs int64, scrapeTime time.Time) pcommon.Timestamp {
	if timestampMs == 0 {
		return pcommon.NewTimestampFromTime(scrapeTime)
	}

	t := time.Unix(0, timestampMs*int64(time.Millisecond))
	return pcommon.NewTimestampFromTime(t)
}

func normalizeName(name string) string {
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) && name != suffix {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return name
}
