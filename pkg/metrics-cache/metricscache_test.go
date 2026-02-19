// Copyright IBM Corp. 2022, 2025
// SPDX-License-Identifier: MPL-2.0

package metricscache

import (
	"sync"
	"testing"
	"time"

	"github.com/armon/go-metrics"
	"github.com/stretchr/testify/require"
)

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
	sink.SetSink(realSink)
	sink.IncrCounter([]string{"mycounter"}, 32)

	data := realSink.Data()

	// Check before the interval is up if setting values will increase the metrics

	mygauge := data[0].Gauges["mygauge"]
	require.EqualValues(t, mygauge.Value, 1)

	// Samples based off setting twice: once at 10, once at 110
	mysamples := data[0].Samples["mysample"]
	require.EqualValues(t, 2, mysamples.Count)
	require.EqualValues(t, 110, mysamples.Sum)
	require.EqualValues(t, 10, mysamples.Min)
	require.EqualValues(t, 100, mysamples.Max)

	// Keys based off a single value of 3
	mykey := data[0].Points["mykey"]
	require.EqualValues(t, mykey[0], 3)

	// counter's based off being set four times with values 4, 8, 16, 32
	mycounter := data[0].Counters["mycounter"]
	require.EqualValues(t, 4, mycounter.Count)
	require.EqualValues(t, 60, mycounter.Sum)
	require.EqualValues(t, 4, mycounter.Min)
	require.EqualValues(t, 32, mycounter.Max)

	sink.IncrCounter([]string{"mycounter"}, 2)
	data = realSink.Data()
	mycounter = data[0].Counters["mycounter"]
	require.EqualValues(t, 5, mycounter.Count)
	require.EqualValues(t, 62, mycounter.Sum)
	require.EqualValues(t, 2, mycounter.Min)
	require.EqualValues(t, 32, mycounter.Max)

}

func TestMetricsCache_ParallelTest(t *testing.T) {
	sink := NewSink()
	// make interval so big we never get metrics split into multiple intervals
	realSink := metrics.NewInmemSink(time.Second, time.Second)

	wg := sync.WaitGroup{}
	wg.Add(4)
	go func() {
		for i := 0; i < 100; i++ {
			sink.SetGauge([]string{"mygauge"}, float32(i+1))
		}
		wg.Done()
	}()

	go func() {
		for i := 0; i < 100; i++ {
			sink.AddSample([]string{"mysample"}, 1)
		}
		wg.Done()
	}()

	go func() {
		for i := 0; i < 100; i++ {
			sink.EmitKey([]string{"mykey"}, 1)
		}
		wg.Done()
	}()

	go func() {
		for i := 0; i < 100; i++ {
			sink.IncrCounter([]string{"mycounter"}, 1)
		}
		wg.Done()
	}()

	sink.SetSink(realSink)
	wg.Wait()

	data := realSink.Data()

	require.NotEmpty(t, data)

	mygauge := data[0].Gauges["mygauge"]
	mysamples := data[0].Samples["mysample"]
	mykey := data[0].Points["mykey"]
	mycounter := data[0].Counters["mycounter"]
	require.EqualValues(t, 100, mysamples.Count)
	require.EqualValues(t, 100, mygauge.Value)

	require.EqualValues(t, 100, len(mykey))
	require.EqualValues(t, 100, mycounter.Count)

}
