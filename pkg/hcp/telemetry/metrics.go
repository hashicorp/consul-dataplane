package telemetry

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	prompb "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var parserPool = sync.Pool{
	New: func() any {
		return new(expfmt.TextParser)
	},
}

type scraper struct {
	logger             hclog.Logger
	envoyAdminHostPort string
	client             http.RoundTripper
}

func (s *scraper) scrapeURLs(filters []string) []string {
	urls := make([]string, len(filters))
	for i, f := range filters {
		urls[i] = fmt.Sprintf(
			"http://%s/stats/prometheus?%s", s.envoyAdminHostPort,
			url.Values(map[string][]string{
				"filter": {f},
			}).Encode())
	}
	return urls
}

func (s *scraper) scrape(ctx context.Context, filters []string, labels pcommon.Map) (pmetric.Metrics, error) {
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	labels.CopyTo(rm.Resource().Attributes())
	scope := rm.ScopeMetrics().AppendEmpty()
	for _, url := range s.scrapeURLs(filters) {
		mfs, err := s.scrapeURL(ctx, url)
		if err != nil {
			return pmetric.Metrics{}, err
		}

		handleMetrics(mfs, scope)
	}

	return metrics, nil
}

func (s *scraper) scrapeURL(ctx context.Context, url string) (map[string]*prompb.MetricFamily, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			s.logger.Warn("failed to close metrics request", "error", err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("non 200 status code %d", resp.StatusCode)
	}

	parser := parserPool.Get().(*expfmt.TextParser)
	defer parserPool.Put(parser)
	return parser.TextToMetricFamilies(resp.Body)
}

func handleMetrics(promMetrics map[string]*prompb.MetricFamily, scopedMetrics pmetric.ScopeMetrics) {
	for _, family := range promMetrics {
		metric := scopedMetrics.Metrics().AppendEmpty()
		metric.SetName(normalizeName(family.GetName()))
		metric.SetDescription(family.GetHelp())
		switch family.GetType() {
		case prompb.MetricType_COUNTER:
			setCounter(family, metric)
		case prompb.MetricType_GAUGE:
			setGauge(family, metric)
		case prompb.MetricType_HISTOGRAM:
			setHistogram(family, metric)
		}
	}
}

func setCounter(family *prompb.MetricFamily, otlpMetric pmetric.Metric) {
	emptySum := otlpMetric.SetEmptySum()
	emptySum.SetIsMonotonic(true)
	emptySum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	for _, metric := range family.GetMetric() {
		dp := emptySum.DataPoints().AppendEmpty()

		for _, labelPair := range metric.GetLabel() {
			dp.Attributes().PutStr(labelPair.GetName(), labelPair.GetValue())
		}

		dp.SetTimestamp(timestampFromMs(metric.GetTimestampMs()))
		dp.SetDoubleValue(metric.GetCounter().GetValue())
	}
}

func setGauge(family *prompb.MetricFamily, otlpMetric pmetric.Metric) {
	emptyGauge := otlpMetric.SetEmptyGauge()
	for _, metric := range family.GetMetric() {
		dp := emptyGauge.DataPoints().AppendEmpty()

		for _, labelPair := range metric.GetLabel() {
			dp.Attributes().PutStr(labelPair.GetName(), labelPair.GetValue())
		}

		dp.SetTimestamp(timestampFromMs(metric.GetTimestampMs()))
		dp.SetDoubleValue(metric.GetGauge().GetValue())
	}
}

func setHistogram(family *prompb.MetricFamily, otlpMetric pmetric.Metric) {
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

		dp.SetTimestamp(timestampFromMs(metric.GetTimestampMs()))
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

func timestampFromMs(timestampMs int64) pcommon.Timestamp {
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
