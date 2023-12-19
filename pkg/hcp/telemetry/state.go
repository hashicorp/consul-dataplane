package telemetry

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/hashicorp/consul/proto-public/pbresource"
	"github.com/hashicorp/go-hclog"
)

type State struct {
	ResourceID   string
	ClientID     string
	ClientSecret string
	Endpoint     string
	Labels       map[string]string
	Metrics      MetricsState
}

type MetricsState struct {
	Endpoint    string
	IncludeList []string
	Disabled    bool
}

func (s State) MetricsEndpoint() string {
	if s.Metrics.Endpoint != "" {
		return s.Metrics.Endpoint
	}
	return s.Endpoint + "/v1/metrics"
}

type StateTracker struct {
	client pbresource.ResourceServiceClient
	logger hclog.Logger

	stateMu sync.Mutex
	state   *State
}

func NewStateTracker(client pbresource.ResourceServiceClient, logger hclog.Logger) *StateTracker {
	return &StateTracker{
		client: client,
		logger: logger,
	}
}

func (st *StateTracker) Run(ctx context.Context) <-chan struct{} {
	notifyCh := make(chan struct{}, 1)
	go st.runLoop(ctx, notifyCh)
	return notifyCh
}

func (st *StateTracker) runLoop(ctx context.Context, notifyCh chan struct{}) {
	watchCh := make(chan *pbresource.WatchEvent, 1)
	go st.watchResource(ctx, watchCh)
	for {
		select {
		case <-ctx.Done():
			return
		case watchEv := <-watchCh:
			st.stateMu.Lock()
			switch watchEv.Operation {
			case pbresource.WatchEvent_OPERATION_UPSERT:
				st.state = resourceToState(watchEv.GetResource())
			case pbresource.WatchEvent_OPERATION_DELETE:
				st.state = nil
			}
			st.stateMu.Unlock()

			select {
			case <-ctx.Done():
				return
			case notifyCh <- struct{}{}:
			default:
			}
		}
	}
}

func (st *StateTracker) State() (state State, exists bool) {
	st.stateMu.Lock()
	if st.state != nil {
		state = *st.state
		exists = true
	}
	st.stateMu.Unlock()
	return state, exists
}

func resourceToState(res *pbresource.Resource) *State {
	if res == nil {
		return nil
	}
	return nil
}

func (st *StateTracker) watchResource(ctx context.Context, ch chan *pbresource.WatchEvent) {
	for {
		stream, err := st.client.WatchList(ctx, &pbresource.WatchListRequest{
			Type: &pbresource.Type{
				Group:        "hcp",
				GroupVersion: "v1",
				Kind:         "TelemetryState",
			},
			Tenancy:    &pbresource.Tenancy{},
			NamePrefix: "default",
		})
		if err != nil {
			st.logger.Error("failed to watch hcp.v1.TelemetryState resource", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}
		st.watchStream(stream, ch)
	}
}

func (st *StateTracker) watchStream(stream pbresource.ResourceService_WatchListClient, ch chan *pbresource.WatchEvent) {
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			st.logger.Error("error handling WatchList stream fo hcp.v1.TelemetryState resource", "error", err)
			return
		}

		// In rare cases where watch events are produced faster than can be processed it is only required to consume
		// the latest event. A single element buffered channel is used to queue the next event for consumption. If
		// the channel is full and sending an event to the channel would block the event in the channel is popped
		// and the latest event is sent. Since this is the only location that sends events to the channel it should
		// in theory never block.
		select {
		case <-stream.Context().Done():
			return
		case ch <- ev:
		default:
			select {
			case <-stream.Context().Done():
				return
			case <-ch:
				ch <- ev
			default:
				ch <- ev
			}
		}
	}
}
