package consuldp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func validConfig() *Config {
	return &Config{
		Consul: &ConsulConfig{
			Addresses: "consul.servers.dns.com",
			GRPCPort:  1234,
			Credentials: &CredentialsConfig{
				Type: CredentialsTypeStatic,
				Static: StaticCredentialsConfig{
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
		XDSServer: &XDSServer{
			BindAddress: "127.0.0.1",
		},
		DNSServer: &DNSServerConfig{
			BindAddr: "127.0.0.1",
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
	require.Nil(t, consulDP.serverConn)
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
		{
			name:      "missing xds bind address",
			modFn:     func(c *Config) { c.XDSServer.BindAddress = "" },
			expectErr: "envoy xDS bind address not specified",
		},
		{
			name: "non-local xds bind address",
			modFn: func(c *Config) {
				c.XDSServer.BindAddress = "1.2.3.4"
			},
			expectErr: "non-local xDS bind address not allowed",
		},
		{
			name: "non-local xds bind address",
			modFn: func(c *Config) {
				c.DNSServer.BindAddr = "1.2.3.4"
				c.DNSServer.Port = 1
			},
			expectErr: "non-local DNS proxy bind address not allowed",
		},
		{
			name: "no bearer token or path given",
			modFn: func(c *Config) {
				c.Consul.Credentials.Type = CredentialsTypeLogin
				c.Consul.Credentials.Login = LoginCredentialsConfig{}
			},
			expectErr: "bearer token (or path to a file containing a bearer token) is required for login",
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
