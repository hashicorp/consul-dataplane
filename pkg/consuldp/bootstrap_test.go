package consuldp

import (
	"bytes"
	"context"
	"flag"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/hashicorp/consul-dataplane/internal/consul-proto/pbdataplane"
)

var (
	update   = flag.Bool("update", false, "update golden files")
	validate = flag.Bool("validate", false, "validate the generated configuration against Envoy")
)

func TestBootstrapConfig(t *testing.T) {
	const (
		serverAddr = "1.2.3.4"
		nodeName   = "agentless-node"
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
				Consul: &ConsulConfig{
					GRPCPort: 1234,
					Credentials: &CredentialsConfig{
						Static: &StaticCredentialsConfig{
							Token: "some-acl-token",
						},
					},
				},
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
				Consul: &ConsulConfig{
					GRPCPort: 1234,
					Credentials: &CredentialsConfig{
						Static: &StaticCredentialsConfig{
							Token: "some-acl-token",
						},
					},
				},
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
			},
			&pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
				Config: makeStruct(map[string]any{
					"envoy_dogstatsd_url": "udp://127.0.0.1:9125",
				}),
			},
		},
		"ready-listener": {
			&Config{
				Consul: &ConsulConfig{
					GRPCPort: 1234,
					Credentials: &CredentialsConfig{
						Static: &StaticCredentialsConfig{
							Token: "some-acl-token",
						},
					},
				},
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
			},
			&pbdataplane.GetEnvoyBootstrapParamsResponse{
				Service:  "web",
				NodeName: nodeName,
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
				consulServer:    &consulServer{address: net.IPAddr{IP: net.ParseIP(serverAddr)}},
			}

			bsCfg, err := dp.bootstrapConfig(ctx)
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
