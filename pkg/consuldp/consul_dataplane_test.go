// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validConfig() *Config {
	return &Config{
		Mode: ModeTypeSidecar,
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
		Proxy: &ProxyConfig{
			NodeName: "agentless-node",
			ProxyID:  "web-proxy",
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
		Telemetry: &TelemetryConfig{
			UseCentralConfig: true,
			Prometheus: PrometheusTelemetryConfig{
				ScrapePath:        "/metrics",
				RetentionTime:     30 * time.Second,
				CACertsPath:       "/tmp/my-certs/",
				KeyFile:           "/tmp/my-key.pem",
				CertFile:          "/tmp/my-cert.pem",
				ServiceMetricsURL: "http://127.0.0.1:12345/metrics",
			},
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
			name:      "missing proxy config",
			modFn:     func(c *Config) { c.Proxy = nil },
			expectErr: "proxy details not specified",
		},
		{
			name:      "missing proxy id",
			modFn:     func(c *Config) { c.Proxy.ProxyID = "" },
			expectErr: "proxy ID not specified",
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
			name:      "missing prometheus ca certs path",
			modFn:     func(c *Config) { c.Telemetry.Prometheus.CACertsPath = "" },
			expectErr: "Must provide -telemetry-prom-ca-certs-path, -telemetry-prom-cert-file, and -telemetry-prom-key-file to enable TLS for prometheus metrics",
		},
		{
			name:      "missing prometheus key file",
			modFn:     func(c *Config) { c.Telemetry.Prometheus.KeyFile = "" },
			expectErr: "Must provide -telemetry-prom-ca-certs-path, -telemetry-prom-cert-file, and -telemetry-prom-key-file to enable TLS for prometheus metrics",
		},
		{
			name:      "missing prometheus cert file",
			modFn:     func(c *Config) { c.Telemetry.Prometheus.CertFile = "" },
			expectErr: "Must provide -telemetry-prom-ca-certs-path, -telemetry-prom-cert-file, and -telemetry-prom-key-file to enable TLS for prometheus metrics",
		},
		{
			name:      "missing prometheus retention time",
			modFn:     func(c *Config) { c.Telemetry.Prometheus.RetentionTime = 0 },
			expectErr: "-telemetry-prom-retention-time must be greater than zero",
		},
		{
			name:      "missing prometheus scrape path",
			modFn:     func(c *Config) { c.Telemetry.Prometheus.ScrapePath = "" },
			expectErr: "-telemetry-prom-scrape-path must not be empty",
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
			expectErr: "non-local DNS proxy bind address not allowed when running as a sidecar",
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
