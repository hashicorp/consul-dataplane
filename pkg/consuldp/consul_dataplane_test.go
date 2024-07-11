// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validConfig(mode ModeType) *Config {
	return &Config{
		Mode: mode,
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
	cfg := validConfig(ModeTypeSidecar)
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
		mode      ModeType
	}

	testCases := []testCase{
		// Side car test cases
		{
			name:      "sidecar mode - missing consul config",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Consul = nil },
			expectErr: "consul addresses not specified",
		},
		{
			name:      "sidecar mode - missing consul addresses",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Consul.Addresses = "" },
			expectErr: "consul addresses not specified",
		},
		{
			name:      "sidecar mode - missing consul server grpc port",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Consul.GRPCPort = 0 },
			expectErr: "consul server gRPC port not specified",
		},
		{
			name:      "sidecar mode - missing proxy config",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Proxy = nil },
			expectErr: "proxy details not specified",
		},
		{
			name:      "sidecar mode - missing proxy id",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Proxy.ProxyID = "" },
			expectErr: "proxy ID not specified",
		},
		{
			name:      "sidecar mode - missing envoy config",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Envoy = nil },
			expectErr: "envoy settings not specified",
		},
		{
			name:      "sidecar mode - missing envoy admin bind address",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Envoy.AdminBindAddress = "" },
			expectErr: "envoy admin bind address not specified",
		},
		{
			name:      "sidecar mode - missing envoy admin bind port",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Envoy.AdminBindPort = 0 },
			expectErr: "envoy admin bind port not specified",
		},
		{
			name:      "sidecar mode - missing logging config",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Logging = nil },
			expectErr: "logging settings not specified",
		},
		{
			name:      "sidecar mode - missing prometheus ca certs path",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Telemetry.Prometheus.CACertsPath = "" },
			expectErr: "Must provide -telemetry-prom-ca-certs-path, -telemetry-prom-cert-file, and -telemetry-prom-key-file to enable TLS for prometheus metrics",
		},
		{
			name:      "sidecar mode - missing prometheus key file",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Telemetry.Prometheus.KeyFile = "" },
			expectErr: "Must provide -telemetry-prom-ca-certs-path, -telemetry-prom-cert-file, and -telemetry-prom-key-file to enable TLS for prometheus metrics",
		},
		{
			name:      "sidecar mode - missing prometheus cert file",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Telemetry.Prometheus.CertFile = "" },
			expectErr: "Must provide -telemetry-prom-ca-certs-path, -telemetry-prom-cert-file, and -telemetry-prom-key-file to enable TLS for prometheus metrics",
		},
		{
			name:      "sidecar mode - missing prometheus retention time",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Telemetry.Prometheus.RetentionTime = 0 },
			expectErr: "-telemetry-prom-retention-time must be greater than zero",
		},
		{
			name:      "sidecar mode - missing prometheus scrape path",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.Telemetry.Prometheus.ScrapePath = "" },
			expectErr: "-telemetry-prom-scrape-path must not be empty",
		},
		{
			name:      "sidecar mode - missing xds bind address",
			mode:      ModeTypeSidecar,
			modFn:     func(c *Config) { c.XDSServer.BindAddress = "" },
			expectErr: "envoy xDS bind address not specified",
		},
		{
			name: "sidecar mode - non-local xds bind address",
			mode: ModeTypeSidecar,
			modFn: func(c *Config) {
				c.XDSServer.BindAddress = "1.2.3.4"
			},
			expectErr: "non-local xDS bind address not allowed",
		},
		{
			name: "sidecar mode - non-local xds bind address",
			mode: ModeTypeSidecar,
			modFn: func(c *Config) {
				c.DNSServer.BindAddr = "1.2.3.4"
				c.DNSServer.Port = 1
			},
			expectErr: "non-local DNS proxy bind address not allowed when running as a sidecar",
		},
		{
			name: "sidecar mode - no bearer token or path given",
			mode: ModeTypeSidecar,
			modFn: func(c *Config) {
				c.Consul.Credentials.Type = CredentialsTypeLogin
				c.Consul.Credentials.Login = LoginCredentialsConfig{}
			},
			expectErr: "bearer token (or path to a file containing a bearer token) is required for login",
		},
	}

	dnsProxyTestCases := []testCase{
		// dns proxy test cases
		{
			name:      "dns-proxy mode - missing consul config",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Consul = nil },
			expectErr: "consul addresses not specified",
		},
		{
			name:      "dns-proxy mode - missing consul addresses",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Consul.Addresses = "" },
			expectErr: "consul addresses not specified",
		},
		{
			name:      "dns-proxy mode - missing consul server grpc port",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Consul.GRPCPort = 0 },
			expectErr: "consul server gRPC port not specified",
		},
		{
			name:      "dns-proxy mode - no error when missing proxy config",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Proxy = nil },
			expectErr: "",
		},
		{
			name:      "dns-proxy mode - no error when missing proxy id",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Proxy.ProxyID = "" },
			expectErr: "",
		},
		{
			name:      "dns-proxy mode - no error when missing envoy config",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Envoy = nil },
			expectErr: "",
		},
		{
			name:      "dns-proxy mode - no error when missing envoy admin bind address",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Envoy.AdminBindAddress = "" },
			expectErr: "",
		},
		{
			name:      "dns-proxy mode - no error when missing envoy admin bind port",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Envoy.AdminBindPort = 0 },
			expectErr: "",
		},
		{
			name:      "dns-proxy mode - missing logging config",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Logging = nil },
			expectErr: "logging settings not specified",
		},
		{
			name:      "dns-proxy mode - missing prometheus ca certs path",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Telemetry.Prometheus.CACertsPath = "" },
			expectErr: "Must provide -telemetry-prom-ca-certs-path, -telemetry-prom-cert-file, and -telemetry-prom-key-file to enable TLS for prometheus metrics",
		},
		{
			name:      "dns-proxy mode - missing prometheus key file",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Telemetry.Prometheus.KeyFile = "" },
			expectErr: "Must provide -telemetry-prom-ca-certs-path, -telemetry-prom-cert-file, and -telemetry-prom-key-file to enable TLS for prometheus metrics",
		},
		{
			name:      "dns-proxy mode - missing prometheus cert file",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Telemetry.Prometheus.CertFile = "" },
			expectErr: "Must provide -telemetry-prom-ca-certs-path, -telemetry-prom-cert-file, and -telemetry-prom-key-file to enable TLS for prometheus metrics",
		},
		{
			name:      "dns-proxy mode - missing prometheus retention time",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Telemetry.Prometheus.RetentionTime = 0 },
			expectErr: "-telemetry-prom-retention-time must be greater than zero",
		},
		{
			name:      "dns-proxy mode - missing prometheus scrape path",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.Telemetry.Prometheus.ScrapePath = "" },
			expectErr: "-telemetry-prom-scrape-path must not be empty",
		},
		{
			name:      "dns-proxy mode - no error when missing xds bind address",
			mode:      ModeTypeDNSProxy,
			modFn:     func(c *Config) { c.XDSServer.BindAddress = "" },
			expectErr: "",
		},
		{
			name: "dns-proxy mode - no error when non-local xds bind address",
			mode: ModeTypeDNSProxy,
			modFn: func(c *Config) {
				c.XDSServer.BindAddress = "1.2.3.4"
			},
			expectErr: "",
		},
		{
			name: "dns-proxy mode - non-local xds bind address",
			mode: ModeTypeDNSProxy,
			modFn: func(c *Config) {
				c.DNSServer.BindAddr = "1.2.3.4"
				c.DNSServer.Port = 1
			},
			expectErr: "",
		},
		{
			name: "dns-proxy mode - no bearer token or path given",
			mode: ModeTypeDNSProxy,
			modFn: func(c *Config) {
				c.Consul.Credentials.Type = CredentialsTypeLogin
				c.Consul.Credentials.Login = LoginCredentialsConfig{}
			},
			expectErr: "bearer token (or path to a file containing a bearer token) is required for login",
		},
	}

	testCases = append(testCases, dnsProxyTestCases...)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig(tc.mode)
			tc.modFn(cfg)

			_, err := NewConsulDP(cfg)
			if tc.expectErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.expectErr)
		})
	}
}
