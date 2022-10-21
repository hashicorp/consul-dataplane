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
		cfg *Config
		rsp *pbdataplane.GetEnvoyBootstrapParamsResponse
	}{
		"basic": {
			&Config{
				Service: &ServiceConfig{
					ServiceID: "web-proxy",
					NodeName:  nodeName,
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
			&pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
				Config: makeStruct(map[string]any{
					"envoy_dogstatsd_url": "this-should-not-appear-in-generated-config",
				}),
			},
		},
		"central-telemetry-config": {
			&Config{
				Service: &ServiceConfig{
					ServiceID: "web-proxy",
					NodeName:  nodeName,
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
			&pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
				Config: makeStruct(map[string]any{
					"envoy_dogstatsd_url": "udp://127.0.0.1:9125",
				}),
			},
		},
		"custom-prometheus-scrape-path": {
			&Config{
				Service: &ServiceConfig{
					ServiceID: "web-proxy",
					NodeName:  nodeName,
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
			&pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
				Config: makeStruct(map[string]any{
					"envoy_prometheus_bind_addr": "0.0.0.0:20200",
				}),
			},
		},
		"ready-listener": {
			&Config{
				Service: &ServiceConfig{
					ServiceID: "web-proxy",
					NodeName:  nodeName,
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
			&pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
			},
		},
		"unix-socket-xds-server": {
			&Config{
				Service: &ServiceConfig{
					ServiceID: "web-proxy",
					NodeName:  nodeName,
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
			&pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
				Config: makeStruct(map[string]any{
					"envoy_dogstatsd_url": "this-should-not-appear-in-generated-config",
				}),
			},
		},
	}
	for desc, tc := range testCases {
		t.Run(desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			client := NewMockDataplaneServiceClient(t)
			client.EXPECT().
				GetEnvoyBootstrapParams(mock.Anything, &pbdataplane.GetEnvoyBootstrapParamsRequest{
					NodeSpec:  &pbdataplane.GetEnvoyBootstrapParamsRequest_NodeName{NodeName: tc.cfg.Service.NodeName},
					ServiceId: tc.cfg.Service.ServiceID,
				}).Call.
				Return(tc.rsp, nil)

			dp := &ConsulDataplane{
				cfg:             tc.cfg,
				dpServiceClient: client,
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
		})
	}
}

func golden(t *testing.T, actual []byte) {
	t.Helper()

	goldenPath := filepath.Join("testdata", t.Name()+".golden")

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
