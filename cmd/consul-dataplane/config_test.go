package main

import (
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/consul-dataplane/pkg/consuldp"
	"github.com/stretchr/testify/require"
)

func TestConfigGeneration(t *testing.T) {
	type testCase struct {
		desc            string
		flagOpts        func() *FlagOpts
		writeConfigFile func() error
		assertConfig    func(cfg *consuldp.Config, f *FlagOpts) bool
		cleanup         func()
		wantErr         bool
	}

	testCases := []testCase{
		{
			desc: "able to generate config properly when the config file input is empty",
			flagOpts: func() *FlagOpts {
				return generateFlagOpts()
			},
			assertConfig: func(cfg *consuldp.Config, flagOpts *FlagOpts) bool {
				expectedCfg := &consuldp.Config{
					Consul: &consuldp.ConsulConfig{
						Addresses:           flagOpts.addresses,
						GRPCPort:            flagOpts.grpcPort,
						ServerWatchDisabled: true,
						Credentials: &consuldp.CredentialsConfig{
							Type: "static",
							Static: consuldp.StaticCredentialsConfig{
								Token: "test-token-123",
							},
							Login: consuldp.LoginCredentialsConfig{
								Meta:        make(map[string]string),
								AuthMethod:  "test-iam-auth",
								BearerToken: "bearer-login",
							},
						},
						TLS: &consuldp.TLSConfig{
							Disabled:           false,
							CACertsPath:        "/consul/",
							CertFile:           "ca-cert.pem",
							KeyFile:            "key.pem",
							ServerName:         "tls-server-name",
							InsecureSkipVerify: true,
						},
					},
					Service: &consuldp.ServiceConfig{
						NodeName:  "test-node-dc1",
						NodeID:    "dc1.node.id",
						Namespace: "default",
						ServiceID: "node1.service1",
						Partition: "default",
					},
					Logging: &consuldp.LoggingConfig{
						Name:     "consul-dataplane",
						LogJSON:  true,
						LogLevel: "WARN",
					},
					DNSServer: &consuldp.DNSServerConfig{
						BindAddr: "127.0.0.1",
						Port:     8604,
					},
					XDSServer: &consuldp.XDSServer{
						BindAddress: "127.0.1.0",
						BindPort:    0,
					},
					Envoy: &consuldp.EnvoyConfig{
						AdminBindAddress:              "127.0.1.0",
						AdminBindPort:                 18000,
						ReadyBindAddress:              "127.0.1.0",
						ReadyBindPort:                 18003,
						EnvoyConcurrency:              4,
						EnvoyDrainStrategy:            "test-strategy",
						ShutdownDrainListenersEnabled: true,
						GracefulShutdownPath:          "/graceful_shutdown",
						EnvoyDrainTimeSeconds:         30,
						GracefulPort:                  20300,
						ExtraArgs:                     []string{},
					},
					Telemetry: &consuldp.TelemetryConfig{
						UseCentralConfig: true,
						Prometheus: consuldp.PrometheusTelemetryConfig{
							RetentionTime: 10 * time.Second,
							ScrapePath:    "/metrics",
							MergePort:     12000,
							CACertsPath:   "/consul/",
							CertFile:      "prom-ca-cert.pem",
							KeyFile:       "prom-key.pem",
						},
					},
				}

				return reflect.DeepEqual(cfg, expectedCfg)
			},
			wantErr: false,
		},
		{
			desc: "able to override all the config fields with CLI flags",
			flagOpts: func() *FlagOpts {
				opts := generateFlagOpts()
				opts.loginBearerTokenPath = "/consul/bearertokenpath/"
				opts.loginDatacenter = "dc100"
				opts.loginMeta = map[string]string{
					"key-1": "value-1",
					"key-2": "value-2",
				}
				opts.loginNamespace = "default"
				opts.loginPartition = "default"

				opts.logJSON = false
				opts.consulDNSBindAddr = "127.0.0.2"
				opts.xdsBindPort = 6060
				opts.dumpEnvoyConfigOnExitEnabled = true
				return opts
			},
			assertConfig: func(cfg *consuldp.Config, flagOpts *FlagOpts) bool {
				expectedCfg := &consuldp.Config{
					Consul: &consuldp.ConsulConfig{
						Addresses:           flagOpts.addresses,
						GRPCPort:            flagOpts.grpcPort,
						ServerWatchDisabled: true,
						Credentials: &consuldp.CredentialsConfig{
							Type: "static",
							Static: consuldp.StaticCredentialsConfig{
								Token: "test-token-123",
							},
							Login: consuldp.LoginCredentialsConfig{
								Meta: map[string]string{
									"key-1": "value-1",
									"key-2": "value-2",
								},
								AuthMethod:      "test-iam-auth",
								BearerToken:     "bearer-login",
								BearerTokenPath: "/consul/bearertokenpath/",
								Namespace:       "default",
								Partition:       "default",
								Datacenter:      "dc100",
							},
						},
						TLS: &consuldp.TLSConfig{
							Disabled:           false,
							CACertsPath:        "/consul/",
							CertFile:           "ca-cert.pem",
							KeyFile:            "key.pem",
							ServerName:         "tls-server-name",
							InsecureSkipVerify: true,
						},
					},
					Service: &consuldp.ServiceConfig{
						NodeName:  "test-node-dc1",
						NodeID:    "dc1.node.id",
						Namespace: "default",
						ServiceID: "node1.service1",
						Partition: "default",
					},
					Logging: &consuldp.LoggingConfig{
						Name:     "consul-dataplane",
						LogJSON:  false,
						LogLevel: "WARN",
					},
					DNSServer: &consuldp.DNSServerConfig{
						BindAddr: "127.0.0.2",
						Port:     8604,
					},
					XDSServer: &consuldp.XDSServer{
						BindAddress: "127.0.1.0",
						BindPort:    6060,
					},
					Envoy: &consuldp.EnvoyConfig{
						AdminBindAddress:              "127.0.1.0",
						AdminBindPort:                 18000,
						ReadyBindAddress:              "127.0.1.0",
						ReadyBindPort:                 18003,
						EnvoyConcurrency:              4,
						EnvoyDrainStrategy:            "test-strategy",
						ShutdownDrainListenersEnabled: true,
						GracefulShutdownPath:          "/graceful_shutdown",
						EnvoyDrainTimeSeconds:         30,
						GracefulPort:                  20300,
						DumpEnvoyConfigOnExitEnabled:  true,
						ExtraArgs:                     []string{},
					},
					Telemetry: &consuldp.TelemetryConfig{
						UseCentralConfig: true,
						Prometheus: consuldp.PrometheusTelemetryConfig{
							RetentionTime: 10 * time.Second,
							ScrapePath:    "/metrics",
							MergePort:     12000,
							CACertsPath:   "/consul/",
							CertFile:      "prom-ca-cert.pem",
							KeyFile:       "prom-key.pem",
						},
					},
				}

				return reflect.DeepEqual(cfg, expectedCfg)
			},
			wantErr: false,
		},
		{
			desc: "able to generate config properly when config file is given without flag inputs",
			flagOpts: func() *FlagOpts {
				opts := &FlagOpts{}
				opts.configFile = "test.json"
				return opts
			},
			writeConfigFile: func() error {
				inputJson := `{
					"consul": {
					  "addresses": "consul_server.dc1",
					  "grpcPort": 8502,
					  "serverWatchDisabled": false
					},
					"service": {
					  "nodeName": "test-node-1",
					  "serviceId": "frontend-service-sidecar-proxy",
					  "namespace": "default",
					  "partition": "default"
					},
					"envoy": {
					  "adminBindAddress": "127.0.0.1",
					  "adminBindPort": 19000
					},
					"logging": {
					  "logLevel": "info",
					  "logJSON": false
					}
				  }`

				err := os.WriteFile("test.json", []byte(inputJson), 0600)
				if err != nil {
					return err
				}
				return nil
			},
			assertConfig: func(cfg *consuldp.Config, flagOpts *FlagOpts) bool {
				expectedCfg := buildDefaultConsulDPConfig()
				expectedCfg.Consul.Addresses = "consul_server.dc1"
				expectedCfg.Consul.GRPCPort = 8502
				expectedCfg.Consul.ServerWatchDisabled = false
				expectedCfg.Service.NodeName = "test-node-1"
				expectedCfg.Service.ServiceID = "frontend-service-sidecar-proxy"
				expectedCfg.Service.Namespace = "default"
				expectedCfg.Service.Partition = "default"
				expectedCfg.Envoy.AdminBindAddress = "127.0.0.1"
				expectedCfg.Envoy.AdminBindPort = 19000
				expectedCfg.Logging.LogJSON = false
				expectedCfg.Logging.LogLevel = "INFO"
				expectedCfg.Telemetry.UseCentralConfig = false

				return reflect.DeepEqual(cfg.Telemetry, expectedCfg.Telemetry)
			},
			cleanup: func() {
				os.Remove("test.json")
			},
			wantErr: false,
		},
		{
			desc: "test whether CLI flag values override the file values",
			flagOpts: func() *FlagOpts {
				opts := generateFlagOpts()
				opts.configFile = "test.json"

				opts.logLevel = "info"
				opts.logJSON = false

				return opts
			},
			writeConfigFile: func() error {
				inputJson := `{
					"consul": {
					  "addresses": "consul_server.dc1",
					  "grpcPort": 8502,
					  "serverWatchDisabled": false
					},
					"service": {
					  "nodeName": "test-node-1",
					  "serviceId": "frontend-service-sidecar-proxy",
					  "namespace": "default",
					  "partition": "default"
					},
					"envoy": {
					  "adminBindAddress": "127.0.0.1",
					  "adminBindPort": 19000
					},
					"logging": {
					  "logLevel": "warn",
					  "logJSON": true
					}
				  }`

				err := os.WriteFile("test.json", []byte(inputJson), 0600)
				if err != nil {
					return err
				}
				return nil
			},
			assertConfig: func(cfg *consuldp.Config, flagOpts *FlagOpts) bool {
				expectedCfg := &consuldp.Config{
					Consul: &consuldp.ConsulConfig{
						Addresses:           flagOpts.addresses,
						GRPCPort:            flagOpts.grpcPort,
						ServerWatchDisabled: true,
						Credentials: &consuldp.CredentialsConfig{
							Type: "static",
							Static: consuldp.StaticCredentialsConfig{
								Token: "test-token-123",
							},
							Login: consuldp.LoginCredentialsConfig{
								Meta: map[string]string{
									"key-1": "value-1",
									"key-2": "value-2",
								},
								AuthMethod:      "test-iam-auth",
								BearerToken:     "bearer-login",
								BearerTokenPath: "/consul/bearertokenpath/",
								Namespace:       "default",
								Partition:       "default",
								Datacenter:      "dc100",
							},
						},
						TLS: &consuldp.TLSConfig{
							Disabled:           false,
							CACertsPath:        "/consul/",
							CertFile:           "ca-cert.pem",
							KeyFile:            "key.pem",
							ServerName:         "tls-server-name",
							InsecureSkipVerify: true,
						},
					},
					Service: &consuldp.ServiceConfig{
						NodeName:  "test-node-dc1",
						NodeID:    "dc1.node.id",
						Namespace: "default",
						ServiceID: "node1.service1",
						Partition: "default",
					},
					Logging: &consuldp.LoggingConfig{
						Name:     "consul-dataplane",
						LogJSON:  true,
						LogLevel: "INFO",
					},
					DNSServer: &consuldp.DNSServerConfig{
						BindAddr: "127.0.0.2",
						Port:     8604,
					},
					XDSServer: &consuldp.XDSServer{
						BindAddress: "127.0.1.0",
						BindPort:    6060,
					},
					Envoy: &consuldp.EnvoyConfig{
						AdminBindAddress:              "127.0.1.0",
						AdminBindPort:                 18000,
						ReadyBindAddress:              "127.0.1.0",
						ReadyBindPort:                 18003,
						EnvoyConcurrency:              4,
						EnvoyDrainStrategy:            "test-strategy",
						ShutdownDrainListenersEnabled: true,
						GracefulShutdownPath:          "/graceful_shutdown",
						EnvoyDrainTimeSeconds:         30,
						GracefulPort:                  20300,
						DumpEnvoyConfigOnExitEnabled:  true,
						ExtraArgs:                     []string{},
					},
					Telemetry: &consuldp.TelemetryConfig{
						UseCentralConfig: true,
						Prometheus: consuldp.PrometheusTelemetryConfig{
							RetentionTime: 10 * time.Second,
							ScrapePath:    "/metrics",
							MergePort:     12000,
							CACertsPath:   "/consul/",
							CertFile:      "prom-ca-cert.pem",
							KeyFile:       "prom-key.pem",
						},
					},
				}

				return reflect.DeepEqual(cfg.Telemetry, expectedCfg.Telemetry)
			},
			cleanup: func() {
				os.Remove("test.json")
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			opts := tc.flagOpts()

			if tc.writeConfigFile != nil {
				require.NoError(t, tc.writeConfigFile())
			}

			cfg, err := opts.buildDataplaneConfig()

			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.True(t, tc.assertConfig(cfg, opts))
			}

			if tc.cleanup != nil {
				tc.cleanup()
			}
		})
	}
}

func generateFlagOpts() *FlagOpts {
	return &FlagOpts{
		addresses:           fmt.Sprintf("consul.address.server_%d", rand.Int()),
		grpcPort:            rand.Int(),
		serverWatchDisabled: true,

		tlsDisabled:           false,
		tlsCACertsPath:        "/consul/",
		tlsCertFile:           "ca-cert.pem",
		tlsKeyFile:            "key.pem",
		tlsServerName:         "tls-server-name",
		tlsInsecureSkipVerify: true,

		logLevel: "warn",
		logJSON:  true,

		nodeName:      "test-node-dc1",
		nodeID:        "dc1.node.id",
		namespace:     "default",
		serviceID:     "node1.service1",
		serviceIDPath: "/consul/service-id",
		partition:     "default",

		credentialType:   "static",
		token:            "test-token-123",
		loginAuthMethod:  "test-iam-auth",
		loginBearerToken: "bearer-login",

		adminBindAddr:                 "127.0.1.0",
		adminBindPort:                 18000,
		readyBindAddr:                 "127.0.1.0",
		readyBindPort:                 18003,
		envoyConcurrency:              4,
		envoyDrainStrategy:            "test-strategy",
		shutdownDrainListenersEnabled: true,

		xdsBindAddr:   "127.0.1.0",
		consulDNSPort: 8604,

		promMergePort: 12000,

		useCentralTelemetryConfig: true,
		promRetentionTime:         10 * time.Second,
		promCACertsPath:           "/consul/",
		promCertFile:              "prom-ca-cert.pem",
		promKeyFile:               "prom-key.pem",
	}
}
