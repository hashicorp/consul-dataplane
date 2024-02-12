package telemetry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/consul-dataplane/internal/mocks/pbresourcemock"
	hcp_v2 "github.com/hashicorp/consul/proto-public/pbhcp/v2"
	pbresource "github.com/hashicorp/consul/proto-public/pbresource"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

func Test_stateTracker(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		modTelemetryState func(*hcp_v2.TelemetryState)
		modResource       func(*pbresource.Resource)
		fail              string // cases for mocks
		event             string
		expectedState     bool
		expectedDisabled  bool
	}{
		"success": {
			event:         "upsert",
			expectedState: true,
		},
		"success no auth url": {
			modTelemetryState: func(ts *hcp_v2.TelemetryState) {
				ts.HcpConfig.AuthUrl = ""
			},
			event:         "upsert",
			expectedState: true,
		},
		"success delete": {
			event: "delete",
		},
		"success despite initial watch list failure": {
			fail:          "WatchListOnce",
			event:         "upsert",
			expectedState: true,
		},
		"success despite initial stream recv failure": {
			fail:          "Recv",
			event:         "upsert",
			expectedState: true,
		},
		"success disabled": {
			modTelemetryState: func(ts *hcp_v2.TelemetryState) {
				ts.Metrics.Disabled = true
			},
			event:            "upsert",
			expectedState:    true,
			expectedDisabled: true,
		},
		"success nil resource": {
			modResource: func(r *pbresource.Resource) {
				r.Data = nil
			},
			event: "upsert",
		},
		"fail watch list": {
			fail:  "WatchList", // the consul WatchList call fails
			event: "unspecified",
		},
		"fail unknown operation": {
			event: "unspecified",
		},
		"fail empty resource": {
			fail:  "NilResourceData",
			event: "upsert",
		},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create a fake state response.
			resourceState := &hcp_v2.TelemetryState{
				ResourceId:   "resource-id",
				ClientId:     "client-id",
				ClientSecret: "client-secret",
				HcpConfig: &hcp_v2.HCPConfig{
					AuthUrl: "https://auth.idp.hashicorp.com",
				},
				Metrics: &hcp_v2.MetricsConfig{
					Labels: map[string]string{
						"foo": "bar",
					},
					Endpoint:    "https://example-endpoint.com",
					IncludeList: []string{".+"},
					Disabled:    false,
				},
			}
			if tc.modTelemetryState != nil {
				tc.modTelemetryState(resourceState)
			}

			data, err := resourceState.MarshalBinary()
			r.NoError(err)

			// Create a fake resource.
			resource := &pbresource.Resource{
				Data: &anypb.Any{
					TypeUrl: "hashicorp.consul.hcp.v2.TelemetryState",
					Value:   data,
				},
			}
			if tc.modResource != nil {
				tc.modResource(resource)
			}
			event := &pbresource.WatchEvent{}
			switch tc.event {
			case "upsert":
				event.Event = &pbresource.WatchEvent_Upsert_{
					Upsert: &pbresource.WatchEvent_Upsert{
						Resource: resource,
					},
				}
			case "delete":
				event.Event = &pbresource.WatchEvent_Delete_{
					Delete: &pbresource.WatchEvent_Delete{
						Resource: resource,
					},
				}
			case "unspecified":
				break
			default:
				r.Fail("unknown event: %q", event)
			}

			// Set up mocks.
			watchListM := pbresourcemock.NewResourceService_WatchListClient(t)
			resourceM := pbresourcemock.NewResourceServiceClient(t)
			func() {
				// Call WatchList on consul.
				if tc.fail == "WatchList" {
					// Constant failures prevent state setting. This repeatedly fails until context is cancelled.
					resourceM.On("WatchList", mock.Anything, mock.Anything).Return(nil, errors.New("boom"))
					go func() {
						time.Sleep(time.Millisecond * 50)
						cancel()
					}()
					return
				}
				if tc.fail == "WatchListOnce" {
					// This is a single failure. Because the tracker retries, subsequent calls with succeed.
					resourceM.On("WatchList", mock.Anything, mock.Anything).Once().Return(nil, errors.New("boom"))
				}
				resourceM.On("WatchList", mock.Anything, &pbresource.WatchListRequest{
					Type:       hcp_v2.TelemetryStateType,
					Tenancy:    &pbresource.Tenancy{},
					NamePrefix: "global",
				}).Return(watchListM, nil)

				// Return a resource on Recv call.
				watchListM.On("Context").Return(ctx)
				if tc.fail == "NilResourceData" {
					watchListM.On("Recv").Return(&pbresource.WatchEvent{
						Event: &pbresource.WatchEvent_Upsert_{
							Upsert: &pbresource.WatchEvent_Upsert{
								Resource: &pbresource.Resource{Data: nil},
							},
						},
					}, nil)
					return
				}
				if tc.fail == "Recv" {
					// This is a single failure. Because the tracker retries, subsequent calls with succeed.
					watchListM.On("Recv").Once().Return(nil, errors.New("boom"))
				}
				watchListM.On("Recv").Return(event, nil)
			}()

			// Create tracker.
			tracker := newStateTracker(resourceM, hclog.NewNullLogger())
			tracker.(*stateTrackerImpl).watchListErrRetryInterval = time.Millisecond * 10
			_, ok := tracker.GetState()
			r.False(ok) // hasn't started yet

			go tracker.Run(ctx)
			select {
			case <-time.After(tracker.(*stateTrackerImpl).watchListErrRetryInterval * 5):
			case <-ctx.Done():
			}

			// Check state.
			state, ok := tracker.GetState()
			if tc.expectedState {
				r.True(ok)
				r.NotNil(state)
				r.Equal(tc.expectedDisabled, state.disabled)
			} else {
				r.False(ok)
			}
		})
	}
}

func Test_getAuthEndpoint(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	r.Equal("https://auth.idp.hcp.dev", getAuthEndpoint("https://consul-telemetry.hcp.dev/otlp/v1/metrics"))
	r.Equal("https://auth.idp.hcp.to", getAuthEndpoint("https://consul-telemetry.hcp.to/otlp/v1/metrics"))
	r.Equal("https://auth.idp.hashicorp.com", getAuthEndpoint("https://consul-telemetry.cloud.hashicorp.com/otlp/v1/metrics"))
}
