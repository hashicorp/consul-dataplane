package metricscache

import (
	"sync"
	"sync/atomic"

	"github.com/armon/go-metrics"
)

type metric struct {
	key    []string
	val    float32
	labels []metrics.Label
}

// Sink is a temporary sink that caches metrics until a real sink is set in SetSink.
// it implements the metrics.MetricSink interface
type Sink struct {
	gauges   []metric
	counters []metric
	samples  []metric
	keys     []metric

	realSink metrics.MetricSink

	mu        sync.Mutex
	checkLock *atomic.Bool
	once      sync.Once
}

// NewSink returns a pointer to a sink with empty cache
func NewSink() *Sink {
	checkLock := &atomic.Bool{}
	checkLock.Store(true)

	return &Sink{
		gauges:    []metric{},
		counters:  []metric{},
		samples:   []metric{},
		keys:      []metric{},
		mu:        sync.Mutex{}, // this lock is used to control access around the slices of metrics above
		checkLock: checkLock,    // we only need to check the lock if we haven't yet set the real sink
	}
}

// SetGauge defaults to SetGaugeWithLabels
func (s *Sink) SetGauge(key []string, val float32) {
	s.SetGaugeWithLabels(key, val, nil)
}

// SetGaugeWithLabels sends metrics to the real sink otherwise caches them
func (s *Sink) SetGaugeWithLabels(key []string, val float32, labels []metrics.Label) {
	if ok := s.checkLock.Load(); ok {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

	if s.realSink != nil {
		s.realSink.SetGaugeWithLabels(key, val, labels)
		return
	}

	s.gauges = append(s.gauges, metric{key: key, val: val, labels: labels})
}

// EmitKey sends metrics to the real sink otherwise caches them
func (s *Sink) EmitKey(key []string, val float32) {
	if ok := s.checkLock.Load(); ok {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

	if s.realSink != nil {
		s.realSink.EmitKey(key, val)
		return
	}

	s.keys = append(s.keys, metric{key: key, val: val})
}

// IncrCounter defaults to IncrCounterWithLabels
func (s *Sink) IncrCounter(key []string, val float32) {
	s.IncrCounterWithLabels(key, val, nil)

}

// IncrCounterWithLabels sends metrics to the real sink otherwise caches them
func (s *Sink) IncrCounterWithLabels(key []string, val float32, labels []metrics.Label) {
	if ok := s.checkLock.Load(); ok {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

	if s.realSink != nil {
		s.realSink.IncrCounterWithLabels(key, val, labels)
		return
	}
	s.counters = append(s.counters, metric{key: key, val: val, labels: labels})
}

// AddSample defaults to AddSampleWithLabels
func (s *Sink) AddSample(key []string, val float32) {
	s.AddSampleWithLabels(key, val, nil)
}

// AddSampleWithLabels sends metrics to the real sink otherwise caches them
func (s *Sink) AddSampleWithLabels(key []string, val float32, labels []metrics.Label) {
	if ok := s.checkLock.Load(); ok {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

	if s.realSink != nil {
		s.realSink.AddSampleWithLabels(key, val, labels)
		return
	}
	s.samples = append(s.samples, metric{key: key, val: val, labels: labels})
}

// SetSink takes a sink and will ensure that the sink sets the value
// and then starts forwarding metrics on to the realSink once called.
// It will also replay all the cached metrics and send them to the realSink
func (s *Sink) SetSink(newSink metrics.MetricSink) {
	s.once.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.checkLock.Store(false)
		s.realSink = newSink
		s.replay()
	})
}

// replay will send cached metrics to the realSink. Once done it will empty the cached store.
func (s *Sink) replay() {
	if s.realSink != nil {
		for _, sample := range s.samples {
			s.realSink.AddSampleWithLabels(sample.key, sample.val, sample.labels)
		}
		s.samples = []metric{} // empty out after replaying samples
		for _, gauge := range s.gauges {
			s.realSink.SetGaugeWithLabels(gauge.key, gauge.val, gauge.labels)
		}
		s.gauges = []metric{} // empty out after replaying gauges
		for _, counter := range s.counters {
			s.realSink.IncrCounterWithLabels(counter.key, counter.val, counter.labels)
		}
		s.counters = []metric{} // empty out after replaying counters
		for _, key := range s.keys {
			s.realSink.EmitKey(key.key, key.val)
		}
		s.keys = []metric{} // empty out after replaying keys
	}
}
