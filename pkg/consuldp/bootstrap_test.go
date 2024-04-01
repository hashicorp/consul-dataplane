// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/consul/proto-public/pbdataplane"
	pbmesh "github.com/hashicorp/consul/proto-public/pbmesh/v2beta1"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

var (
	update   = flag.Bool("update", false, "update golden files")
	validate = flag.Bool("validate", false, "validate the generated configuration against Envoy")
)

func TestBootstrapConfig(t *testing.T) {
	const (
		nodeName    = "agentless-node"
		xdsBindPort = 1234
		socketPath  = "/var/run/xds.sock"
	)

	makeStruct := func(kv map[string]any) *structpb.Struct {
		s, err := structpb.NewStruct(kv)
		require.NoError(t, err)
		return s
	}

	testCases := map[string]struct {
		cfg                 *Config
		rsp                 *pbdataplane.GetEnvoyBootstrapParamsResponse
		rspV2               *pbdataplane.GetEnvoyBootstrapParamsResponse
		resolvedProxyConfig *ProxyConfig
	}{
		"access-logs": {
			cfg: &Config{
				Proxy: &ProxyConfig{
					ProxyID:  "web-proxy",
					NodeName: nodeName,
				},
				Envoy: &EnvoyConfig{
					AdminBindAddress: "127.0.0.1",
					AdminBindPort:    19000,
				},
				Telemetry: &TelemetryConfig{
					UseCentralConfig: false,
				},
				XDSServer: &XDSServer{BindAddress: "127.0.0.1", BindPort: xdsBindPort},
			},
			rsp: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
				Config: makeStruct(map[string]any{
					"envoy_dogstatsd_url": "this-should-not-appear-in-generated-config",
				}),
				AccessLogs: []string{"{\"name\":\"Consul Listener Filter Log\",\"typedConfig\":{\"@type\":\"type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog\",\"logFormat\":{\"jsonFormat\":{\"custom_field\":\"%START_TIME%\"}}}}"},
			},
			rspV2: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Identity: "web",
				NodeName: nodeName,
				BootstrapConfig: &pbmesh.BootstrapConfig{
					DogstatsdUrl: "this-should-not-appear-in-generated-config",
				},
				AccessLogs: []string{"{\"name\":\"Consul Listener Filter Log\",\"typedConfig\":{\"@type\":\"type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog\",\"logFormat\":{\"jsonFormat\":{\"custom_field\":\"%START_TIME%\"}}}}"},
			},
		},
		"basic": {
			cfg: &Config{
				Proxy: &ProxyConfig{
					ProxyID:  "web-proxy",
					NodeName: nodeName,
				},
				Envoy: &EnvoyConfig{
					AdminBindAddress: "127.0.0.1",
					AdminBindPort:    19000,
				},
				Telemetry: &TelemetryConfig{
					UseCentralConfig: false,
				},
				XDSServer: &XDSServer{BindAddress: "127.0.0.1", BindPort: xdsBindPort},
			},
			rsp: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
				Config: makeStruct(map[string]any{
					"envoy_dogstatsd_url": "this-should-not-appear-in-generated-config",
				}),
			},
			rspV2: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Identity: "web",
				NodeName: nodeName,
				BootstrapConfig: &pbmesh.BootstrapConfig{
					DogstatsdUrl: "this-should-not-appear-in-generated-config",
				},
			},
		},
		"central-telemetry-config": {
			cfg: &Config{
				Proxy: &ProxyConfig{
					ProxyID:  "web-proxy",
					NodeName: nodeName,
				},
				Envoy: &EnvoyConfig{
					AdminBindAddress: "127.0.0.1",
					AdminBindPort:    19000,
				},
				Telemetry: &TelemetryConfig{
					UseCentralConfig: true,
				},
				XDSServer: &XDSServer{BindAddress: "127.0.0.1", BindPort: xdsBindPort},
			},
			rsp: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
				Config: makeStruct(map[string]any{
					"envoy_dogstatsd_url": "udp://127.0.0.1:9125",
				}),
			},
			rspV2: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Identity: "web",
				NodeName: nodeName,
				BootstrapConfig: &pbmesh.BootstrapConfig{
					DogstatsdUrl: "udp://127.0.0.1:9125",
				},
			},
		},
		"hcp-metrics": {
			cfg: &Config{
				Proxy: &ProxyConfig{
					ProxyID:   "web-proxy",
					NodeName:  nodeName,
					Namespace: "default",
				},
				Envoy: &EnvoyConfig{
					AdminBindAddress: "127.0.0.1",
					AdminBindPort:    19000,
				},
				Telemetry: &TelemetryConfig{
					UseCentralConfig: true,
				},
				XDSServer: &XDSServer{BindAddress: "127.0.0.1", BindPort: xdsBindPort},
			},
			rsp: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:   "web",
				Namespace: "default",
				NodeName:  nodeName,
				Config: makeStruct(map[string]any{
					"envoy_telemetry_collector_bind_socket_dir": "/tmp/consul/hcp-metrics",
				}),
			},
			rspV2: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Identity:  "web",
				Namespace: "default",
				NodeName:  nodeName,
				BootstrapConfig: &pbmesh.BootstrapConfig{
					TelemetryCollectorBindSocketDir: "/tmp/consul/hcp-metrics",
				},
			},
		},
		"custom-prometheus-scrape-path": {
			cfg: &Config{
				Proxy: &ProxyConfig{
					ProxyID:  "web-proxy",
					NodeName: nodeName,
				},
				Envoy: &EnvoyConfig{
					AdminBindAddress: "127.0.0.1",
					AdminBindPort:    19000,
				},
				Telemetry: &TelemetryConfig{
					UseCentralConfig: true,
					Prometheus: PrometheusTelemetryConfig{
						MergePort:  20100,
						ScrapePath: "/custom/scrape/path",
					},
				},
				XDSServer: &XDSServer{BindAddress: "127.0.0.1", BindPort: xdsBindPort},
			},
			rsp: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
				Config: makeStruct(map[string]any{
					"envoy_prometheus_bind_addr": "0.0.0.0:20200",
				}),
			},
			rspV2: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Identity: "web",
				NodeName: nodeName,
				BootstrapConfig: &pbmesh.BootstrapConfig{
					PrometheusBindAddr: "0.0.0.0:20200",
				},
			},
		},
		"custom-prometheus-scrape-path-with-query": {
			cfg: &Config{
				Proxy: &ProxyConfig{
					ProxyID:  "web-proxy",
					NodeName: nodeName,
				},
				Envoy: &EnvoyConfig{
					AdminBindAddress: "127.0.0.1",
					AdminBindPort:    19000,
				},
				Telemetry: &TelemetryConfig{
					UseCentralConfig: true,
					Prometheus: PrometheusTelemetryConfig{
						MergePort: 20100,
						// Expect query is _not_ included in xDS path match
						ScrapePath: "/custom/scrape/path?usedonly",
					},
				},
				XDSServer: &XDSServer{BindAddress: "127.0.0.1", BindPort: xdsBindPort},
			},
			rsp: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
				Config: makeStruct(map[string]any{
					"envoy_prometheus_bind_addr": "0.0.0.0:20200",
				}),
			},
			rspV2: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Identity: "web",
				NodeName: nodeName,
				BootstrapConfig: &pbmesh.BootstrapConfig{
					PrometheusBindAddr: "0.0.0.0:20200",
				},
			},
		},
		"non-default tenancy": {
			cfg: &Config{
				Proxy: &ProxyConfig{
					ProxyID:  "web-proxy",
					NodeName: nodeName,
					// No tenancy provided here to make sure it comes from the bootstrap call
				},
				Envoy: &EnvoyConfig{
					AdminBindAddress: "127.0.0.1",
					AdminBindPort:    19000,
				},
				Telemetry: &TelemetryConfig{
					UseCentralConfig: false,
				},
				XDSServer: &XDSServer{BindAddress: "127.0.0.1", BindPort: xdsBindPort},
			},
			rsp: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
				Config: makeStruct(map[string]any{
					"envoy_dogstatsd_url": "this-should-not-appear-in-generated-config",
				}),
				Namespace: "test-namespace",
				Partition: "test-partition",
			},
			rspV2: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Identity: "web",
				NodeName: nodeName,
				BootstrapConfig: &pbmesh.BootstrapConfig{
					DogstatsdUrl: "this-should-not-appear-in-generated-config",
				},
				Namespace: "test-namespace",
				Partition: "test-partition",
			},
			// We want to ensure cdp is configured with the resolved tenancy
			resolvedProxyConfig: &ProxyConfig{
				NodeName:  nodeName,
				ProxyID:   "web-proxy",
				Namespace: "test-namespace",
				Partition: "test-partition",
			},
		},
		"ready-listener": {
			cfg: &Config{
				Proxy: &ProxyConfig{
					ProxyID:  "web-proxy",
					NodeName: nodeName,
				},
				Envoy: &EnvoyConfig{
					AdminBindAddress: "127.0.0.1",
					AdminBindPort:    19000,
					ReadyBindAddress: "127.0.0.1",
					ReadyBindPort:    20000,
				},
				Telemetry: &TelemetryConfig{
					UseCentralConfig: false,
				},
				XDSServer: &XDSServer{BindAddress: "127.0.0.1", BindPort: xdsBindPort},
			},
			rsp: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
			},
			rspV2: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Identity: "web",
				NodeName: nodeName,
			},
		},
		"unix-socket-xds-server": {
			cfg: &Config{
				Proxy: &ProxyConfig{
					ProxyID:  "web-proxy",
					NodeName: nodeName,
				},
				Envoy: &EnvoyConfig{
					AdminBindAddress: "127.0.0.1",
					AdminBindPort:    19000,
				},
				Telemetry: &TelemetryConfig{
					UseCentralConfig: false,
				},
				XDSServer: &XDSServer{BindAddress: fmt.Sprintf("unix://%s", socketPath)},
			},
			rsp: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
				Config: makeStruct(map[string]any{
					"envoy_dogstatsd_url": "this-should-not-appear-in-generated-config",
				}),
			},
			rspV2: &pbdataplane.GetEnvoyBootstrapParamsResponse{
				Identity: "web",
				NodeName: nodeName,
				BootstrapConfig: &pbmesh.BootstrapConfig{
					DogstatsdUrl: "this-should-not-appear-in-generated-config",
				},
			},
		},
	}
	for desc, tc := range testCases {
		t.Run(desc+"-v1", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			client := NewMockDataplaneServiceClient(t)
			client.EXPECT().
				GetEnvoyBootstrapParams(mock.Anything, &pbdataplane.GetEnvoyBootstrapParamsRequest{
					NodeSpec:  &pbdataplane.GetEnvoyBootstrapParamsRequest_NodeName{NodeName: tc.cfg.Proxy.NodeName},
					ServiceId: tc.cfg.Proxy.ProxyID,
					ProxyId:   tc.cfg.Proxy.ProxyID,
					Namespace: tc.cfg.Proxy.Namespace,
				}).Call.
				Return(tc.rsp, nil)

			dp := &ConsulDataplane{
				cfg:             tc.cfg,
				dpServiceClient: client,
				logger:          hclog.NewNullLogger(),
			}

			if strings.HasPrefix(tc.cfg.XDSServer.BindAddress, "unix://") {
				dp.xdsServer = &xdsServer{listenerAddress: socketPath, listenerNetwork: "unix"}
			} else {
				dp.xdsServer = &xdsServer{listenerAddress: fmt.Sprintf("127.0.0.1:%d", xdsBindPort)}
			}

			_, bsCfg, err := dp.bootstrapConfig(ctx)
			require.NoError(t, err)

			golden(t, bsCfg)
			validateBootstrapConfig(t, bsCfg)

			if tc.resolvedProxyConfig != nil {
				require.Equal(t, *tc.resolvedProxyConfig, dp.resolvedProxyConfig)
			}
		})

		t.Run(desc+"-v2", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			client := NewMockDataplaneServiceClient(t)
			client.EXPECT().
				GetEnvoyBootstrapParams(mock.Anything, &pbdataplane.GetEnvoyBootstrapParamsRequest{
					NodeSpec:  &pbdataplane.GetEnvoyBootstrapParamsRequest_NodeName{NodeName: tc.cfg.Proxy.NodeName},
					ServiceId: tc.cfg.Proxy.ProxyID,
					ProxyId:   tc.cfg.Proxy.ProxyID,
					Namespace: tc.cfg.Proxy.Namespace,
				}).Call.
				Return(tc.rspV2, nil)

			dp := &ConsulDataplane{
				cfg:             tc.cfg,
				dpServiceClient: client,
				logger:          hclog.NewNullLogger(),
			}

			if strings.HasPrefix(tc.cfg.XDSServer.BindAddress, "unix://") {
				dp.xdsServer = &xdsServer{listenerAddress: socketPath, listenerNetwork: "unix"}
			} else {
				dp.xdsServer = &xdsServer{listenerAddress: fmt.Sprintf("127.0.0.1:%d", xdsBindPort)}
			}

			_, bsCfg, err := dp.bootstrapConfig(ctx)
			require.NoError(t, err)

			golden(t, bsCfg)
			validateBootstrapConfig(t, bsCfg)

			if tc.resolvedProxyConfig != nil {
				require.Equal(t, *tc.resolvedProxyConfig, dp.resolvedProxyConfig)
			}
		})
	}
}

func golden(t *testing.T, actual []byte) {
	t.Helper()

	fileName := strings.TrimSuffix(t.Name(), "-v1")
	fileName = strings.TrimSuffix(fileName, "-v2")

	goldenPath := filepath.Join("testdata", fileName+".golden")

	if *update {
		require.NoError(t, os.WriteFile(goldenPath, actual, 0644))
	} else {
		golden, err := os.ReadFile(goldenPath)
		require.NoError(t, err)
		require.Equal(t, string(golden), string(actual))
	}
}

func validateBootstrapConfig(t *testing.T, cfg []byte) {
	t.Run("validate", func(t *testing.T) {
		if !*validate {
			t.Skip()
		}

		bin, err := exec.LookPath("envoy")
		require.NoError(t, err)

		cmd := exec.Command(bin,
			"--mode", "validate",
			"--config-yaml", string(cfg),
		)

		var b bytes.Buffer
		cmd.Stdout = &b
		cmd.Stderr = &b

		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to validate config: %s", b.String())
		}
	})
}
