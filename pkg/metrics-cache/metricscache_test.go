package metricscache

import (
	"testing"
	"time"

	"github.com/armon/go-metrics"
	"github.com/stretchr/testify/require"
)

func validateMetrics(t *testing.T, data []*metrics.IntervalMetrics) {
	require.NotEmpty(t, data)
	// Gauge based off being set once at value of 1
	mygauge := data[0].Gauges["mygauge"]
	require.EqualValues(t, mygauge.Value, 1)

	// Samples based off setting twice: once at 10, once at 110
	mysamples := data[0].Samples["mysample"]
	require.EqualValues(t, 2, mysamples.AggregateSample.Count)
	require.EqualValues(t, 110, mysamples.AggregateSample.Sum)
	require.EqualValues(t, 10, mysamples.AggregateSample.Min)
	require.EqualValues(t, 100, mysamples.AggregateSample.Max)

	// Keys based off a single value of 3
	mykey := data[0].Points["mykey"]
	require.EqualValues(t, mykey[0], 3)

	// counter's based off being set four times with values 4, 8, 16, 32
	mycounter := data[0].Counters["mycounter"]
	require.EqualValues(t, 4, mycounter.AggregateSample.Count)
	require.EqualValues(t, 60, mycounter.AggregateSample.Sum)
	require.EqualValues(t, 4, mycounter.AggregateSample.Min)
	require.EqualValues(t, 32, mycounter.AggregateSample.Max)

}

func TestMetricsCache_BasicPath(t *testing.T) {
	sink := NewSink()

	sink.SetGauge([]string{"mygauge"}, 1)
	sink.AddSample([]string{"mysample"}, 10)
	sink.AddSample([]string{"mysample"}, 100)
	sink.EmitKey([]string{"mykey"}, 3)
	sink.IncrCounter([]string{"mycounter"}, 4)
	sink.IncrCounter([]string{"mycounter"}, 8)
	sink.IncrCounter([]string{"mycounter"}, 16)

	realSink := metrics.NewInmemSink(time.Second, time.Second*1)
	err := sink.SetSink(realSink)
	require.NoError(t, err)
	sink.IncrCounter([]string{"mycounter"}, 32)

	data := realSink.Data()
	validateMetrics(t, data)

	// Check before the interval is up if setting values will increase the metrics
	sink.IncrCounter([]string{"mycounter"}, 2)
	data = realSink.Data()
	mycounter := data[0].Counters["mycounter"]
	require.EqualValues(t, 5, mycounter.AggregateSample.Count)
	require.EqualValues(t, 62, mycounter.AggregateSample.Sum)
	require.EqualValues(t, 2, mycounter.AggregateSample.Min)
	require.EqualValues(t, 32, mycounter.AggregateSample.Max)

	time.Sleep(time.Second)

	sink.SetGauge([]string{"mygauge"}, 1)
	sink.AddSample([]string{"mysample"}, 10)
	sink.AddSample([]string{"mysample"}, 100)
	sink.EmitKey([]string{"mykey"}, 3)
	sink.IncrCounter([]string{"mycounter"}, 4)
	sink.IncrCounter([]string{"mycounter"}, 8)
	sink.IncrCounter([]string{"mycounter"}, 16)
	sink.IncrCounter([]string{"mycounter"}, 32)

	data = realSink.Data()
	validateMetrics(t, data)
}

func TestMetricsCache_ParallelTest(t *testing.T) {
	sink := NewSink()
	realSink := metrics.NewInmemSink(time.Second, time.Second*20)

	go func() {
		for i := 0; i < 100; i++ {
			sink.SetGauge([]string{"mygauge"}, float32(i))
		}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			sink.AddSample([]string{"mysample"}, 1)
		}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			sink.EmitKey([]string{"mykey"}, 100)
		}
	}()

	go func() {
		t.Logf("starting")
		for i := 0; i < 100; i++ {
			sink.IncrCounter([]string{"mycounter"}, 1)
		}
		t.Logf("done")
	}()

	err := sink.SetSink(realSink)
	require.NoError(t, err)
	time.Sleep(time.Second)

	data := realSink.Data()

	require.NotEmpty(t, data)
	mygauge := data[0].Gauges["mygauge"]
	mysamples := data[0].Samples["mysample"]
	mykey := data[0].Points["mykey"]
	mycounter := data[0].Counters["mycounter"]
	require.NotEmpty(t, mygauge.Value, 100)
	require.NotEmpty(t, mysamples.AggregateSample.Count, 100)
	require.NotEmpty(t, mykey, 1)
	require.NotEmpty(t, mycounter.AggregateSample.Count, 100)

}
