// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul-dataplane/pkg/consuldp"
)

func TestConfigGeneration(t *testing.T) {
	type testCase struct {
		desc            string
		flagOpts        func() (*FlagOpts, error)
		writeConfigFile func(t *testing.T) error
		makeExpectedCfg func(f *FlagOpts) *consuldp.Config
		wantErr         bool
	}

	testCases := []testCase{
		{
			desc: "able to generate config properly when the config file input is empty",
			flagOpts: func() (*FlagOpts, error) {
				return generateFlagOptsWithServiceFlags()
			},
			makeExpectedCfg: func(flagOpts *FlagOpts) *consuldp.Config {
				return &consuldp.Config{
					Mode: consuldp.ModeTypeSidecar,
					Consul: &consuldp.ConsulConfig{
						Addresses:           stringVal(flagOpts.dataplaneConfig.Consul.Addresses),
						GRPCPort:            intVal(flagOpts.dataplaneConfig.Consul.GRPCPort),
						ServerWatchDisabled: true,
						Credentials: &consuldp.CredentialsConfig{
							Type: "static",
							Static: consuldp.StaticCredentialsConfig{
								Token: "test-token-123",
							},
							Login: consuldp.LoginCredentialsConfig{
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
					Proxy: &consuldp.ProxyConfig{
						NodeName:  "test-node-dc1",
						NodeID:    "dc1.node.id",
						Namespace: "default",
						ProxyID:   "node1.service1",
						Partition: "default",
					},
					Logging: &consuldp.LoggingConfig{
						Name:     DefaultLogName,
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
						GracefulStartupPath:           "/graceful_startup",
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
			},
			wantErr: false,
		},
		{
			desc: "able to generate config properly with proxy flags when the config file input is empty",
			flagOpts: func() (*FlagOpts, error) {
				return generateFlagOptsWithProxyFlags()
			},
			makeExpectedCfg: func(flagOpts *FlagOpts) *consuldp.Config {
				return &consuldp.Config{
					Mode: consuldp.ModeTypeSidecar,
					Consul: &consuldp.ConsulConfig{
						Addresses:           stringVal(flagOpts.dataplaneConfig.Consul.Addresses),
						GRPCPort:            intVal(flagOpts.dataplaneConfig.Consul.GRPCPort),
						ServerWatchDisabled: true,
						Credentials: &consuldp.CredentialsConfig{
							Type: "static",
							Static: consuldp.StaticCredentialsConfig{
								Token: "test-token-123",
							},
							Login: consuldp.LoginCredentialsConfig{
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
					Proxy: &consuldp.ProxyConfig{
						NodeName:  "test-node-dc1",
						NodeID:    "dc1.node.id",
						Namespace: "default",
						ProxyID:   "node1.service1",
						Partition: "default",
					},
					Logging: &consuldp.LoggingConfig{
						Name:     DefaultLogName,
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
						GracefulStartupPath:           "/graceful_startup",
						GracefulShutdownPath:          "/graceful_shutdown",
						EnvoyDrainTimeSeconds:         30,
						GracefulPort:                  20300,
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
			},
			wantErr: false,
		},
		{
			desc: "able to override all the config fields with CLI flags when using service flags",
			flagOpts: func() (*FlagOpts, error) {
				opts, err := generateFlagOptsWithServiceFlags()
				if err != nil {
					return nil, err
				}
				opts.dataplaneConfig.Mode = strReference("dns-proxy")
				opts.dataplaneConfig.Consul.Credentials.Login.BearerTokenPath = strReference("/consul/bearertokenpath/")
				opts.dataplaneConfig.Consul.Credentials.Login.Datacenter = strReference("dc100")
				opts.dataplaneConfig.Consul.Credentials.Login.Meta = map[string]string{
					"key-1": "value-1",
					"key-2": "value-2",
				}
				opts.dataplaneConfig.Consul.Credentials.Login.Namespace = strReference("default")
				opts.dataplaneConfig.Consul.Credentials.Login.Partition = strReference("default")

				opts.dataplaneConfig.Logging.LogJSON = boolReference(false)
				opts.dataplaneConfig.DNSServer.BindAddr = strReference("127.0.0.2")
				opts.dataplaneConfig.XDSServer.BindPort = intReference(6060)
				opts.dataplaneConfig.Envoy.DumpEnvoyConfigOnExitEnabled = boolReference(true)

				return opts, nil
			},
			makeExpectedCfg: func(flagOpts *FlagOpts) *consuldp.Config {
				return &consuldp.Config{
					Mode: consuldp.ModeTypeDNSProxy,
					Consul: &consuldp.ConsulConfig{
						Addresses:           stringVal(flagOpts.dataplaneConfig.Consul.Addresses),
						GRPCPort:            intVal(flagOpts.dataplaneConfig.Consul.GRPCPort),
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
					Proxy: &consuldp.ProxyConfig{
						NodeName:  "test-node-dc1",
						NodeID:    "dc1.node.id",
						Namespace: "default",
						ProxyID:   "node1.service1",
						Partition: "default",
					},
					Logging: &consuldp.LoggingConfig{
						Name:     DefaultLogName,
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
						GracefulStartupPath:           "/graceful_startup",
						GracefulShutdownPath:          "/graceful_shutdown",
						EnvoyDrainTimeSeconds:         30,
						GracefulPort:                  20300,
						DumpEnvoyConfigOnExitEnabled:  true,
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
			},
			wantErr: false,
		},
		{
			desc: "able to override all the config fields with CLI flags when using proxy flags",
			flagOpts: func() (*FlagOpts, error) {
				opts, err := generateFlagOptsWithProxyFlags()
				if err != nil {
					return nil, err
				}
				opts.dataplaneConfig.Consul.Credentials.Login.BearerTokenPath = strReference("/consul/bearertokenpath/")
				opts.dataplaneConfig.Consul.Credentials.Login.Datacenter = strReference("dc100")
				opts.dataplaneConfig.Consul.Credentials.Login.Meta = map[string]string{
					"key-1": "value-1",
					"key-2": "value-2",
				}
				opts.dataplaneConfig.Consul.Credentials.Login.Namespace = strReference("default")
				opts.dataplaneConfig.Consul.Credentials.Login.Partition = strReference("default")

				opts.dataplaneConfig.Logging.LogJSON = boolReference(false)
				opts.dataplaneConfig.DNSServer.BindAddr = strReference("127.0.0.2")
				opts.dataplaneConfig.XDSServer.BindPort = intReference(6060)
				opts.dataplaneConfig.Envoy.DumpEnvoyConfigOnExitEnabled = boolReference(true)
				return opts, nil
			},
			makeExpectedCfg: func(flagOpts *FlagOpts) *consuldp.Config {
				return &consuldp.Config{
					Mode: consuldp.ModeTypeSidecar,
					Consul: &consuldp.ConsulConfig{
						Addresses:           stringVal(flagOpts.dataplaneConfig.Consul.Addresses),
						GRPCPort:            intVal(flagOpts.dataplaneConfig.Consul.GRPCPort),
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
					Proxy: &consuldp.ProxyConfig{
						NodeName:  "test-node-dc1",
						NodeID:    "dc1.node.id",
						Namespace: "default",
						ProxyID:   "node1.service1",
						Partition: "default",
					},
					Logging: &consuldp.LoggingConfig{
						Name:     DefaultLogName,
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
						GracefulStartupPath:           "/graceful_startup",
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
			},
			wantErr: false,
		},
		{
			desc: "prefer proxy flags over service flags when both are set",
			flagOpts: func() (*FlagOpts, error) {
				opts, err := generateFlagOptsWithServiceFlags()
				if err != nil {
					return nil, err
				}
				opts.dataplaneConfig.Proxy.ID = strReference("proxy-id")
				opts.dataplaneConfig.Proxy.NodeName = strReference("proxy-node-name")
				opts.dataplaneConfig.Proxy.NodeID = strReference("proxy-node-id")
				opts.dataplaneConfig.Proxy.Namespace = strReference("foo")
				opts.dataplaneConfig.Proxy.Partition = strReference("bar")

				opts.dataplaneConfig.XDSServer.BindPort = intReference(6060)
				return opts, nil
			},
			makeExpectedCfg: func(flagOpts *FlagOpts) *consuldp.Config {
				return &consuldp.Config{
					Mode: consuldp.ModeTypeSidecar,
					Consul: &consuldp.ConsulConfig{
						Addresses:           stringVal(flagOpts.dataplaneConfig.Consul.Addresses),
						GRPCPort:            intVal(flagOpts.dataplaneConfig.Consul.GRPCPort),
						ServerWatchDisabled: true,
						Credentials: &consuldp.CredentialsConfig{
							Type: "static",
							Static: consuldp.StaticCredentialsConfig{
								Token: "test-token-123",
							},
							Login: consuldp.LoginCredentialsConfig{
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
					Proxy: &consuldp.ProxyConfig{
						NodeName:  "proxy-node-name",
						NodeID:    "proxy-node-id",
						Namespace: "foo",
						ProxyID:   "proxy-id",
						Partition: "bar",
					},
					Logging: &consuldp.LoggingConfig{
						Name:     DefaultLogName,
						LogJSON:  true,
						LogLevel: "WARN",
					},
					DNSServer: &consuldp.DNSServerConfig{
						BindAddr: "127.0.0.1",
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
						GracefulStartupPath:           "/graceful_startup",
						EnvoyDrainTimeSeconds:         30,
						GracefulPort:                  20300,
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
			},
			wantErr: false,
		},
		{
			desc: "able to generate config properly when config file is given without flag inputs",
			flagOpts: func() (*FlagOpts, error) {
				opts := &FlagOpts{}
				opts.configFile = "test.json"
				return opts, nil
			},
			writeConfigFile: func(t *testing.T) error {
				inputJson := `{
					"consul": {
					  "addresses": "consul_server.dc1",
					  "grpcPort": 8502,
					  "serverWatchDisabled": false
					},
					"proxy": {
					  "nodeName": "test-node-1",
					  "id": "frontend-service-sidecar-proxy",
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

				t.Cleanup(func() {
					_ = os.Remove("test.json")
				})
				return nil
			},
			makeExpectedCfg: func(flagOpts *FlagOpts) *consuldp.Config {
				return &consuldp.Config{
					Mode: consuldp.ModeTypeSidecar,
					Consul: &consuldp.ConsulConfig{
						Addresses:           "consul_server.dc1",
						GRPCPort:            8502,
						ServerWatchDisabled: false,
						Credentials: &consuldp.CredentialsConfig{
							Static: consuldp.StaticCredentialsConfig{},
							Login:  consuldp.LoginCredentialsConfig{},
						},
						TLS: &consuldp.TLSConfig{},
					},
					Proxy: &consuldp.ProxyConfig{
						NodeName:  "test-node-1",
						Namespace: "default",
						ProxyID:   "frontend-service-sidecar-proxy",
						Partition: "default",
					},
					Logging: &consuldp.LoggingConfig{
						Name:     DefaultLogName,
						LogJSON:  false,
						LogLevel: "INFO",
					},
					DNSServer: &consuldp.DNSServerConfig{
						BindAddr: "127.0.0.1",
						Port:     -1,
					},
					XDSServer: &consuldp.XDSServer{
						BindAddress: "127.0.0.1",
						BindPort:    0,
					},
					Envoy: &consuldp.EnvoyConfig{
						AdminBindAddress:              "127.0.0.1",
						AdminBindPort:                 19000,
						ReadyBindPort:                 0,
						EnvoyConcurrency:              2,
						EnvoyDrainStrategy:            "immediate",
						ShutdownDrainListenersEnabled: false,
						GracefulShutdownPath:          "/graceful_shutdown",
						EnvoyDrainTimeSeconds:         30,
						GracefulPort:                  20300,
						DumpEnvoyConfigOnExitEnabled:  false,
						GracefulStartupPath:           "/graceful_startup",
					},
					Telemetry: &consuldp.TelemetryConfig{
						UseCentralConfig: true,
						Prometheus: consuldp.PrometheusTelemetryConfig{
							RetentionTime: 60 * time.Second,
							ScrapePath:    "/metrics",
							MergePort:     20100,
						},
					},
				}
			},
			wantErr: false,
		},
		{
			desc: "test whether CLI flag values override the file values with service flags",
			flagOpts: func() (*FlagOpts, error) {
				opts, err := generateFlagOptsWithServiceFlags()
				if err != nil {
					return nil, err
				}
				opts.configFile = "test.json"

				opts.dataplaneConfig.Logging.LogLevel = strReference("info")
				opts.dataplaneConfig.Logging.LogJSON = boolReference(false)
				opts.dataplaneConfig.Consul.Credentials.Login.Meta = map[string]string{
					"key1": "value1",
				}

				return opts, nil
			},
			writeConfigFile: func(t *testing.T) error {
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

				t.Cleanup(func() {
					_ = os.Remove("test.json")
				})
				return nil
			},
			makeExpectedCfg: func(flagOpts *FlagOpts) *consuldp.Config {
				return &consuldp.Config{
					Mode: consuldp.ModeTypeSidecar,
					Consul: &consuldp.ConsulConfig{
						Addresses:           stringVal(flagOpts.dataplaneConfig.Consul.Addresses),
						GRPCPort:            intVal(flagOpts.dataplaneConfig.Consul.GRPCPort),
						ServerWatchDisabled: true,
						Credentials: &consuldp.CredentialsConfig{
							Type: "static",
							Static: consuldp.StaticCredentialsConfig{
								Token: "test-token-123",
							},
							Login: consuldp.LoginCredentialsConfig{
								AuthMethod:  "test-iam-auth",
								BearerToken: "bearer-login",
								Meta: map[string]string{
									"key1": "value1",
								},
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
					Proxy: &consuldp.ProxyConfig{
						NodeName:  "test-node-dc1",
						NodeID:    "dc1.node.id",
						Namespace: "default",
						ProxyID:   "node1.service1",
						Partition: "default",
					},
					Logging: &consuldp.LoggingConfig{
						Name:     DefaultLogName,
						LogJSON:  false,
						LogLevel: "INFO",
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
						DumpEnvoyConfigOnExitEnabled:  false,
						GracefulStartupPath:           "/graceful_startup",
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
			},
			wantErr: false,
		},
		{
			desc: "test whether CLI flag values override the file values with proxy flags",
			flagOpts: func() (*FlagOpts, error) {
				opts, err := generateFlagOptsWithProxyFlags()
				opts.dataplaneConfig.Mode = strReference("dns-proxy")
				if err != nil {
					return nil, err
				}
				opts.configFile = "test.json"

				opts.dataplaneConfig.Logging.LogLevel = strReference("info")
				opts.dataplaneConfig.Logging.LogJSON = boolReference(false)
				opts.dataplaneConfig.Consul.Credentials.Login.Meta = map[string]string{
					"key1": "value1",
				}

				return opts, nil
			},
			writeConfigFile: func(t *testing.T) error {
				inputJson := `{
					"consul": {
					  "addresses": "consul_server.dc1",
					  "grpcPort": 8502,
					  "serverWatchDisabled": false
					},
					"proxy": {
					  "nodeName": "test-node-1",
					  "proxyId": "frontend-service-sidecar-proxy",
					  "namespace": "default",
					  "partition": "default"
					},
					"envoy": {
					  "enabled": false,
					  "adminBindAddress": "127.0.0.1",
					  "adminBindPort": 19000
					},
					"xdsServer": {
					  "enabled": false
					},
					"dnsServer": {
					  "enabled": false
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

				t.Cleanup(func() {
					_ = os.Remove("test.json")
				})
				return nil
			},
			makeExpectedCfg: func(flagOpts *FlagOpts) *consuldp.Config {
				return &consuldp.Config{
					Mode: consuldp.ModeTypeDNSProxy,
					Consul: &consuldp.ConsulConfig{
						Addresses:           stringVal(flagOpts.dataplaneConfig.Consul.Addresses),
						GRPCPort:            intVal(flagOpts.dataplaneConfig.Consul.GRPCPort),
						ServerWatchDisabled: true,
						Credentials: &consuldp.CredentialsConfig{
							Type: "static",
							Static: consuldp.StaticCredentialsConfig{
								Token: "test-token-123",
							},
							Login: consuldp.LoginCredentialsConfig{
								AuthMethod:  "test-iam-auth",
								BearerToken: "bearer-login",
								Meta: map[string]string{
									"key1": "value1",
								},
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
					Proxy: &consuldp.ProxyConfig{
						NodeName:  "test-node-dc1",
						NodeID:    "dc1.node.id",
						Namespace: "default",
						ProxyID:   "node1.service1",
						Partition: "default",
					},
					Logging: &consuldp.LoggingConfig{
						Name:     DefaultLogName,
						LogJSON:  false,
						LogLevel: "INFO",
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
						GracefulStartupPath:           "/graceful_startup",
						EnvoyDrainTimeSeconds:         30,
						GracefulPort:                  20300,
						DumpEnvoyConfigOnExitEnabled:  false,
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
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			opts, err := tc.flagOpts()
			require.NoError(t, err)

			if tc.writeConfigFile != nil {
				require.NoError(t, tc.writeConfigFile(t))
			}

			cfg, err := opts.buildDataplaneConfig(nil)

			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				expCfg := tc.makeExpectedCfg(opts)
				require.Equal(t, expCfg, cfg)
			}
		})
	}
}

func generateFlagOptsWithServiceFlags() (*FlagOpts, error) {
	data := `
	{
		"consul": {
			"addresses": "` + fmt.Sprintf("consul.address.server_%d", rand.Int()) + `",
			"grpcPort": ` + fmt.Sprintf("%d", rand.Int()) + `,
			"serverWatchDisabled": true,
			"tls": {
				"disabled": false,
				"caCertsPath": "/consul/",
				"certFile": "ca-cert.pem",
				"keyFile": "key.pem",
				"serverName": "tls-server-name",
				"insecureSkipVerify": true
			},
			"credentials": {
				"type": "static",
				"static": {
					"token": "test-token-123"
				},
				"login": {
					"authMethod": "test-iam-auth",
					"bearerToken": "bearer-login"
				}
			}
		},
		"service": {
			"nodeName": "test-node-dc1",
			"nodeID": "dc1.node.id",
			"namespace": "default",
			"serviceID": "node1.service1",
			"partition": "default"
		},
		"logging": {
			"logJSON": true,
			"logLevel": "warn"
		},
		"telemetry": {
			"useCentralConfig": true,
			"prometheus": {
				"retentionTime": "10s",
				"scrapePath": "/metrics",
				"mergePort": 12000,
				"caCertsPath": "/consul/",
				"certFile": "prom-ca-cert.pem",
				"keyFile": "prom-key.pem"
			}
		},
		"envoy": {
			"adminBindAddress": "127.0.1.0",
			"adminBindPort": 18000,
			"readyBindAddress": "127.0.1.0",
			"readyBindPort": 18003,
			"concurrency": 4,
			"drainStrategy": "test-strategy",
			"shutdownDrainListenersEnabled": true
		},
		"xdsServer": {
			"bindAddress": "127.0.1.0"
		},
		"dnsServer": {
			"bindPort": 8604
		}
	}`

	var configFlags *DataplaneConfigFlags
	err := json.Unmarshal([]byte(data), &configFlags)
	if err != nil {
		return nil, err
	}

	return &FlagOpts{
		dataplaneConfig: *configFlags,
	}, nil
}
func generateFlagOptsWithProxyFlags() (*FlagOpts, error) {
	data := `
	{
		"consul": {
			"addresses": "` + fmt.Sprintf("consul.address.server_%d", rand.Int()) + `",
			"grpcPort": ` + fmt.Sprintf("%d", rand.Int()) + `,
			"serverWatchDisabled": true,
			"tls": {
				"disabled": false,
				"caCertsPath": "/consul/",
				"certFile": "ca-cert.pem",
				"keyFile": "key.pem",
				"serverName": "tls-server-name",
				"insecureSkipVerify": true
			},
			"credentials": {
				"type": "static",
				"static": {
					"token": "test-token-123"
				},
				"login": {
					"authMethod": "test-iam-auth",
					"bearerToken": "bearer-login"
				}
			}
		},
		"proxy": {
			"nodeName": "test-node-dc1",
			"nodeID": "dc1.node.id",
			"namespace": "default",
			"id": "node1.service1",
			"partition": "default"
		},
		"logging": {
			"logJSON": true,
			"logLevel": "warn"
		},
		"telemetry": {
			"useCentralConfig": true,
			"prometheus": {
				"retentionTime": "10s",
				"scrapePath": "/metrics",
				"mergePort": 12000,
				"caCertsPath": "/consul/",
				"certFile": "prom-ca-cert.pem",
				"keyFile": "prom-key.pem"
			}
		},
		"envoy": {
			"adminBindAddress": "127.0.1.0",
			"adminBindPort": 18000,
			"readyBindAddress": "127.0.1.0",
			"readyBindPort": 18003,
			"concurrency": 4,
			"drainStrategy": "test-strategy",
			"shutdownDrainListenersEnabled": true
		},
		"xdsServer": {
			"bindAddress": "127.0.1.0"
		},
		"dnsServer": {
			"bindPort": 8604
		}
	}`

	var configFlags *DataplaneConfigFlags
	err := json.Unmarshal([]byte(data), &configFlags)
	if err != nil {
		return nil, err
	}

	return &FlagOpts{
		dataplaneConfig: *configFlags,
	}, nil
}

func strReference(s string) *string {
	return &s
}

func boolReference(b bool) *bool {
	return &b
}

func intReference(i int) *int {
	return &i
}
