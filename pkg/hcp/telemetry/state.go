// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package telemetry

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/consul-dataplane/pkg/hcp/telemetry/otlphttp"
	"github.com/hashicorp/consul-dataplane/pkg/version"
	hcp_v2 "github.com/hashicorp/consul/proto-public/pbhcp/v2"
	"github.com/hashicorp/consul/proto-public/pbresource"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/hcp-sdk-go/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// stateTrackerWatchListErrRetryInterval is the interval to wait before retrying another WatchList call
	// to Consul after a previous call fails. This is used in an attempt to reduce spamming of Consul
	// during intermittent failures.
	stateTrackerWatchListErrRetryInterval = time.Minute

	// stateTrackerWatchListInvalidErrRetryInterval is the interval to wait before retrying another WatchList call
	// to Consul after a previous call fails with InvalidRequest. This is assumed to happen when
	// running against older versions of Consul that do not have the hcp.v2.TelemetryState resource registered.
	stateTrackerWatchListInvalidErrRetryInterval = time.Hour
)

var (
	errStateTrackerNilResource = errors.New("unexpected nil resource received from WatchList stream for hcp.v2.TelemetryState resource")
)

// state holds the otlp client and labels and metric filters to use when scraping metrics from Envoy and pushing metrics to HCP.
//
// All of this is sourced from Consul that, in turn, gets it from HCP.
type state struct {
	client      otlphttp.Client
	labels      map[string]string
	disabled    bool
	includeList []string
}

type stateTracker interface {
	Run(ctx context.Context)
	GetState() (*state, bool)
}

type stateTrackerImpl struct {
	client pbresource.ResourceServiceClient
	logger hclog.Logger

	// time to wait after a WatchList call fails
	watchListErrRetryInterval time.Duration
	// time to wait after a WatchList call fails with InvalidRequest. This is assumed to happen when
	// running against older versions of Consul that do not have the hcp.v2.TelemetryState resource
	// registered.
	watchListInvalidErrRetryInterval time.Duration

	stateMu sync.Mutex
	state   *state
}

func newStateTracker(client pbresource.ResourceServiceClient, logger hclog.Logger) stateTracker {
	return &stateTrackerImpl{
		client:                           client,
		logger:                           logger,
		watchListErrRetryInterval:        stateTrackerWatchListErrRetryInterval,
		watchListInvalidErrRetryInterval: stateTrackerWatchListInvalidErrRetryInterval,
	}
}

func (st *stateTrackerImpl) GetState() (*state, bool) {
	st.stateMu.Lock()
	defer st.stateMu.Unlock()

	if st.state != nil {
		return st.state, true
	}

	return nil, false
}

func (st *stateTrackerImpl) Run(ctx context.Context) {
	for {
		st.logger.Debug("starting watch for hcp.v2.TelemetryState resource")

		stream, err := st.client.WatchList(ctx, &pbresource.WatchListRequest{
			Type:       hcp_v2.TelemetryStateType,
			Tenancy:    &pbresource.Tenancy{},
			NamePrefix: "global",
		})
		if err != nil {
			// If the error was that the request was invalid, we may be running against a version of Consul without
			// the hcp.v2.TelemetryState resource registered. In this case, we want to wait a long time before retrying.
			retryInterval := st.watchListErrRetryInterval
			if status.Code(err) == codes.InvalidArgument {
				retryInterval = st.watchListInvalidErrRetryInterval
				st.logger.Debug("failed to watch hcp.v2.TelemetryState resource, invalid", "error", err, "retry_interval", retryInterval)
			} else {
				st.logger.Error("failed to watch hcp.v2.TelemetryState resource", "error", err, "retry_interval", retryInterval)
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(retryInterval):
				continue
			}
		}

		if err := st.recvStream(stream); err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(st.watchListErrRetryInterval):
				continue
			}
		}
	}
}

func (st *stateTrackerImpl) recvStream(stream pbresource.ResourceService_WatchListClient) error {
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			return err
		}
		if err != nil {
			st.logger.Error("failed to receive a WatchEvent for the hcp.v2.TelemetryState resource", "error", err)
			return err
		}

		st.stateMu.Lock()
		if ev.GetUpsert() != nil {
			state, err := resourceToState(ev.GetUpsert().GetResource(), st.logger)
			if err != nil {
				st.logger.Error("failed to convert resource to state", "error", err)
			} else {
				st.logger.Info("updated hcp telemetry exporter config")
				st.state = state
			}
		} else if ev.GetDelete() != nil {
			st.logger.Info("hcp.v2.TelemetryState resource deleted, clearing from state")
			st.state = nil
		} else if ev.GetEndOfSnapshot() == nil {
			st.logger.Error("unexpected event operation type received from WatchList stream")
		}
		st.stateMu.Unlock()

		select {
		case <-stream.Context().Done():
			return nil
		default:
		}
	}
}

// resourceToState takes in a an hcp.v2.TelemetryState resource and converts it to a state struct.
func resourceToState(res *pbresource.Resource, logger hclog.Logger) (*state, error) {
	// Get and unmarshal the resource data into a telemetry state.
	data := res.GetData()
	if data == nil {
		return nil, errStateTrackerNilResource
	}

	logger = logger.With("id", res.GetId(), "generation", res.GetGeneration(), "version", res.GetVersion())

	telemetryState := &hcp_v2.TelemetryState{}
	if err := data.UnmarshalTo(telemetryState); err != nil {
		return nil, fmt.Errorf("failed to unmarshal telemetry state resource: %w", err)
	}

	if telemetryState.GetMetrics().GetDisabled() {
		logger.Debug("received telemetry state resource", "disabled", true)
		return &state{
			disabled: true,
		}, nil
	}

	// Get the endpoint for HCP auth. If this is empty, ie not set in any env var in consul, we infer it from
	// the metrics endpoint.
	authURL := telemetryState.GetHcpConfig().GetAuthUrl()
	if authURL == "" {
		authURL = getAuthEndpoint(telemetryState.GetMetrics().GetEndpoint())
		logger = logger.With("hcp_auth_endpoint_inferred", true)
	}

	// Create an HCP configuration.
	hcpConfig, err := config.NewHCPConfig(
		config.WithClientCredentials(telemetryState.GetClientId(), telemetryState.GetClientSecret()),
		config.WithAuth(authURL, &tls.Config{}),
		config.WithoutBrowserLogin(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create hcp config: %w", err)
	}

	// Create an OTLP http client.
	cfg := &otlphttp.Config{
		MetricsEndpoint: telemetryState.GetMetrics().GetEndpoint(),
		TLSConfig:       hcpConfig.APITLSConfig(),
		TokenSource:     hcpConfig,
		Middleware: []otlphttp.MiddlewareOption{otlphttp.WithRequestHeaders(map[string]string{
			"X-HCP-Resource-ID": telemetryState.GetResourceId(),
			"X-Channel":         fmt.Sprintf("consul-dataplane/%s", version.GetHumanVersion()),
		})},
		UserAgent:  fmt.Sprintf("consul-dataplane/%s (%s/%s)", version.GetHumanVersion(), runtime.GOOS, runtime.GOARCH),
		Logger:     logger.Named("otlphttp"),
		HTTPProxy:  telemetryState.GetProxy().GetHttpProxy(),
		HTTPSProxy: telemetryState.GetProxy().GetHttpsProxy(),
		NoProxy:    strings.Join(telemetryState.GetProxy().GetNoProxy(), ","),
	}
	exporter, err := otlphttp.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build otlphttp client: %w", err)
	}

	logger.Debug("received telemetry state resource",
		"disabled", false,
		"hcp_resource_id", telemetryState.GetResourceId(),
		"hcp_client_id", telemetryState.GetClientId(),
		"hcp_auth_endpoint", authURL,
		"labels", telemetryState.GetMetrics().GetLabels(),
		"include_list", telemetryState.GetMetrics().GetIncludeList(),
		"http_proxy", telemetryState.GetProxy().GetHttpProxy(),
		"https_proxy", telemetryState.GetProxy().GetHttpsProxy(),
		"no_proxy", telemetryState.GetProxy().GetNoProxy(),
	)

	return &state{
		client:      exporter,
		labels:      telemetryState.GetMetrics().GetLabels(),
		disabled:    telemetryState.GetMetrics().GetDisabled(),
		includeList: telemetryState.GetMetrics().GetIncludeList(),
	}, nil
}

func getAuthEndpoint(metricsEndpoint string) string {
	switch {
	case strings.Contains(metricsEndpoint, "hcp.dev"):
		return "https://auth.idp.hcp.dev"
	case strings.Contains(metricsEndpoint, "hcp.to"):
		return "https://auth.idp.hcp.to"
	default:
		return "https://auth.idp.hashicorp.com"
	}
}
