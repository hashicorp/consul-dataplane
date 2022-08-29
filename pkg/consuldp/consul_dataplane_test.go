package consuldp

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul-dataplane/internal/consul-proto/pbdataplane"
)

func validConfig() *Config {
	return &Config{
		Consul: &ConsulConfig{
			Addresses: "consul.servers.dns.com",
			GRPCPort:  1234,
			Credentials: &CredentialsConfig{
				Static: &StaticCredentialsConfig{
					Token: "some-acl-token",
				},
			},
		},
		Service: &ServiceConfig{
			NodeName:  "agentless-node",
			ServiceID: "web-proxy",
		},
		Logging: &LoggingConfig{
			LogLevel: "INFO",
		},
		Envoy: &EnvoyConfig{
			AdminBindAddress: "127.0.0.1",
			AdminBindPort:    19000,
		},
	}
}

func TestNewConsulDP(t *testing.T) {
	cfg := validConfig()
	consulDP, err := NewConsulDP(cfg)
	require.NoError(t, err)
	require.NotNil(t, consulDP)
	require.Equal(t, cfg.Logging.Name, consulDP.logger.Name())
	require.True(t, consulDP.logger.IsInfo())
	require.Equal(t, cfg, consulDP.cfg)
	require.Nil(t, consulDP.consulServer)
}

func TestNewConsulDPError(t *testing.T) {
	type testCase struct {
		name      string
		modFn     func(*Config)
		expectErr string
	}

	testCases := []testCase{
		{
			name:      "missing consul config",
			modFn:     func(c *Config) { c.Consul = nil },
			expectErr: "consul addresses not specified",
		},
		{
			name:      "missing consul addresses",
			modFn:     func(c *Config) { c.Consul.Addresses = "" },
			expectErr: "consul addresses not specified",
		},
		{
			name:      "missing consul server grpc port",
			modFn:     func(c *Config) { c.Consul.GRPCPort = 0 },
			expectErr: "consul server gRPC port not specified",
		},
		{
			name:      "missing service config",
			modFn:     func(c *Config) { c.Service = nil },
			expectErr: "service details not specified",
		},
		{
			name: "missing node details",
			modFn: func(c *Config) {
				c.Service.NodeName = ""
				c.Service.NodeID = ""
			},
			expectErr: "node name or ID not specified",
		},
		{
			name: "missing node details",
			modFn: func(c *Config) {
				c.Service.NodeName = ""
				c.Service.NodeID = ""
			},
			expectErr: "node name or ID not specified",
		},
		{
			name:      "missing service id",
			modFn:     func(c *Config) { c.Service.ServiceID = "" },
			expectErr: "proxy service ID not specified",
		},
		{
			name:      "missing envoy config",
			modFn:     func(c *Config) { c.Envoy = nil },
			expectErr: "envoy settings not specified",
		},
		{
			name:      "missing envoy admin bind address",
			modFn:     func(c *Config) { c.Envoy.AdminBindAddress = "" },
			expectErr: "envoy admin bind address not specified",
		},
		{
			name:      "missing envoy admin bind port",
			modFn:     func(c *Config) { c.Envoy.AdminBindPort = 0 },
			expectErr: "envoy admin bind port not specified",
		},
		{
			name:      "missing logging config",
			modFn:     func(c *Config) { c.Logging = nil },
			expectErr: "logging settings not specified",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			tc.modFn(cfg)

			_, err := NewConsulDP(cfg)
			require.EqualError(t, err, tc.expectErr)
		})
	}
}

func TestResolveAndPickConsulServerAddress(t *testing.T) {
	cfg := validConfig()
	cfg.Consul.Addresses = "exec=echo 127.0.0.1"

	consulDP, err := NewConsulDP(cfg)
	require.NoError(t, err)

	require.NoError(t, consulDP.resolveAndPickConsulServerAddress(context.Background()))
	require.Equal(t, net.IPAddr{IP: net.ParseIP("127.0.0.1")}, consulDP.consulServer.address)
}

func TestResolveAndPickConsulServerAddressError(t *testing.T) {
	cfg := validConfig()
	cfg.Consul.Addresses = "invalid-dns"

	consulDP, err := NewConsulDP(cfg)
	require.NoError(t, err)
	require.ErrorContains(t, consulDP.resolveAndPickConsulServerAddress(context.Background()), "failure resolving consul server addresses")
	require.Nil(t, consulDP.consulServer)
}

func TestSetConsulServerSupportedFeatures(t *testing.T) {
	cfg := validConfig()
	consulDP, err := NewConsulDP(cfg)
	require.NoError(t, err)

	consulDP.consulServer = &consulServer{address: net.IPAddr{IP: net.ParseIP("127.0.0.1")}}

	mockDataplaneServiceClient := NewMockDataplaneServiceClient(t)
	consulDP.dpServiceClient = mockDataplaneServiceClient
	supportedFeatures := []*pbdataplane.DataplaneFeatureSupport{
		{
			FeatureName: pbdataplane.DataplaneFeatures_DATAPLANE_FEATURES_WATCH_SERVERS,
			Supported:   true,
		},
		{
			FeatureName: pbdataplane.DataplaneFeatures_DATAPLANE_FEATURES_EDGE_CERTIFICATE_MANAGEMENT,
			Supported:   true,
		},
		{
			FeatureName: pbdataplane.DataplaneFeatures_DATAPLANE_FEATURES_ENVOY_BOOTSTRAP_CONFIGURATION,
			Supported:   true,
		},
	}
	mockDataplaneServiceClient.EXPECT().
		GetSupportedDataplaneFeatures(mock.Anything, mock.Anything, mock.Anything).Call.
		Return(&pbdataplane.GetSupportedDataplaneFeaturesResponse{SupportedDataplaneFeatures: supportedFeatures}, nil)

	err = consulDP.setConsulServerSupportedFeatures(context.Background())
	require.NoError(t, err)
	require.Equal(t, len(supportedFeatures), len(consulDP.consulServer.supportedFeatures))
}

func TestSetConsulServerSupportedFeaturesError(t *testing.T) {
	cfg := validConfig()
	consulDP, err := NewConsulDP(cfg)
	require.NoError(t, err)

	consulDP.consulServer = &consulServer{address: net.IPAddr{IP: net.ParseIP("127.0.0.1")}}

	mockDataplaneServiceClient := NewMockDataplaneServiceClient(t)
	consulDP.dpServiceClient = mockDataplaneServiceClient
	mockDataplaneServiceClient.EXPECT().
		GetSupportedDataplaneFeatures(mock.Anything, mock.Anything, mock.Anything).Call.
		Return(nil, fmt.Errorf("error!"))

	require.ErrorContains(t, consulDP.setConsulServerSupportedFeatures(context.Background()), "failure getting supported consul-dataplane features")
	require.Empty(t, consulDP.consulServer.supportedFeatures)
}
