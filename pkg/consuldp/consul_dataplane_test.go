package consuldp

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/hashicorp/consul-dataplane/internal/consul-proto/pbdataplane"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewConsulDP(t *testing.T) {
	cfg := &Config{
		Consul:  &ConsulConfig{Addresses: "consul.servers.dns.com", GRPCPort: 8502},
		Logging: &LoggingConfig{Name: "consul-dataplane"},
	}
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
		cfg       *Config
		expectErr string
	}

	run := func(t *testing.T, tc testCase) {
		_, err := NewConsulDP(tc.cfg)
		require.EqualError(t, err, tc.expectErr)
	}

	testCases := []testCase{
		{
			name:      "missing consul config",
			cfg:       &Config{},
			expectErr: "consul addresses not specified",
		},
		{
			name:      "missing consul addresses",
			cfg:       &Config{Consul: &ConsulConfig{}},
			expectErr: "consul addresses not specified",
		},
		{
			name:      "missing consul server grpc port",
			cfg:       &Config{Consul: &ConsulConfig{Addresses: "consul.servers.dns.com"}},
			expectErr: "consul server gRPC port not specified",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc)
		})
	}
}

func TestResolveAndPickConsulServerAddress(t *testing.T) {
	cfg := &Config{
		Consul:  &ConsulConfig{Addresses: "exec=echo 127.0.0.1", GRPCPort: 8502},
		Logging: &LoggingConfig{Name: "consul-dataplane"},
	}
	consulDP, err := NewConsulDP(cfg)
	require.NoError(t, err)

	require.NoError(t, consulDP.resolveAndPickConsulServerAddress(context.Background()))
	require.Equal(t, net.IPAddr{IP: net.ParseIP("127.0.0.1")}, consulDP.consulServer.address)
}

func TestResolveAndPickConsulServerAddressError(t *testing.T) {
	cfg := &Config{
		Consul:  &ConsulConfig{Addresses: "invalid-dns", GRPCPort: 8502},
		Logging: &LoggingConfig{Name: "consul-dataplane"},
	}
	consulDP, err := NewConsulDP(cfg)
	require.NoError(t, err)
	require.ErrorContains(t, consulDP.resolveAndPickConsulServerAddress(context.Background()), "failure resolving consul server addresses")
	require.Nil(t, consulDP.consulServer)
}

func TestSetConsulServerSupportedFeatures(t *testing.T) {
	cfg := &Config{
		Consul:  &ConsulConfig{Addresses: "exec=echo 127.0.0.1", GRPCPort: 8502},
		Logging: &LoggingConfig{Name: "consul-dataplane"},
	}
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
	cfg := &Config{
		Consul:  &ConsulConfig{Addresses: "exec=echo 127.0.0.1", GRPCPort: 8502},
		Logging: &LoggingConfig{Name: "consul-dataplane"},
	}
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
