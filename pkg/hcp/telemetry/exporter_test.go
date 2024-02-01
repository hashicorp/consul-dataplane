package telemetry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

type fakeScraper struct {
	scrapeCalled atomic.Bool
	scrapeErr    error
}

func (s *fakeScraper) scrape(ctx context.Context, filters []string, labels pcommon.Map) (pmetric.Metrics, error) {
	s.scrapeCalled.Store(true)

	if s.scrapeErr != nil {
		return pmetric.NewMetrics(), s.scrapeErr
	}

	return pmetric.NewMetrics(), nil
}

type fakeStateTracker struct {
	runCalled      atomic.Bool
	getStateCalled atomic.Bool
	state          *state
}

func (s *fakeStateTracker) Run(ctx context.Context) {
	s.runCalled.Store(true)
}

func (s *fakeStateTracker) GetState() (*state, bool) {
	s.getStateCalled.Store(true)

	if s.state != nil {
		return s.state, true
	}

	return nil, false
}

type fakeClient struct {
	exportCalled atomic.Bool
	exportErr    error
}

func (e *fakeClient) ExportMetrics(ctx context.Context, metrics pmetric.Metrics) error {
	e.exportCalled.Store(true)

	if e.exportErr != nil {
		return e.exportErr
	}

	return nil
}

func Test_Worker(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		modState  func(*state)
		nilState  bool // nil state entirely
		scrapeErr error
		exportErr error

		expectScrape bool // whether we expect to scrape metrics
		expectExport bool // whether we expect to attempt to export metrics
	}{
		"success": {
			expectScrape: true,
			expectExport: true,
		},
		"disabled": {
			modState: func(s *state) {
				s.disabled = true
			},
		},
		"disabled nil state": {
			nilState: true,
		},
		"failure scrape": {
			scrapeErr:    errors.New("failed to scrape"),
			expectScrape: true,
		},
		"failure export": {
			exportErr:    errors.New("failed to export"),
			expectScrape: true,
			expectExport: true,
		},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			// Create a worker. We don't use the client or envoy admin addr for anything.
			worker := New(NewMockResourceServiceClient(t), hclog.NewNullLogger(), "localhost:1234")
			worker.scrapeInterval = time.Millisecond * 10

			// Create fake exporter and state.
			exporter := &fakeClient{
				exportErr: tc.exportErr,
			}
			state := &state{
				client:   exporter,
				disabled: false,
				labels: map[string]string{
					"foo": "bar",
				},
				includeList: []string{"a", "b"},
			}
			if tc.modState != nil {
				tc.modState(state)
			}
			if tc.nilState {
				state = nil
			}

			// Create fake scraper and state tracker.
			scraper := &fakeScraper{
				scrapeErr: tc.scrapeErr,
			}
			stateTracker := &fakeStateTracker{
				state: state,
			}
			worker.scraper = scraper
			worker.stateTracker = stateTracker

			// Run the worker.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go worker.Run(ctx)
			time.Sleep(worker.scrapeInterval * 5)
			cancel()

			r.True(stateTracker.runCalled.Load())
			r.True(stateTracker.getStateCalled.Load())
			r.Equal(tc.expectScrape, scraper.scrapeCalled.Load())
			r.Equal(tc.expectExport, exporter.exportCalled.Load())
		})
	}
}
