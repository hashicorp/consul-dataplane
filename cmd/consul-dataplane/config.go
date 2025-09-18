// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"os"
	"strings"

	"dario.cat/mergo"

	"github.com/hashicorp/consul-dataplane/pkg/consuldp"
)

type FlagOpts struct {
	dataplaneConfig  DataplaneConfigFlags
	checkProxyHealth bool

	printVersion bool
	configFile   string
}

type DataplaneConfigFlags struct {
	Mode      *string        `json:"mode,omitempty"`
	Consul    ConsulFlags    `json:"consul,omitempty"`
	Service   ServiceFlags   `json:"service,omitempty"`
	Proxy     ProxyFlags     `json:"proxy,omitempty"`
	Logging   LogFlags       `json:"logging,omitempty"`
	XDSServer XDSServerFlags `json:"xdsServer,omitempty"`
	DNSServer DNSServerFlags `json:"dnsServer,omitempty"`
	Telemetry TelemetryFlags `json:"telemetry,omitempty"`
	Envoy     EnvoyFlags     `json:"envoy,omitempty"`
}

type ConsulFlags struct {
	Addresses           *string `json:"addresses,omitempty"`
	GRPCPort            *int    `json:"grpcPort,omitempty"`
	ServerWatchDisabled *bool   `json:"serverWatchDisabled,omitempty"`

	TLS         TLSFlags         `json:"tls,omitempty"`
	Credentials CredentialsFlags `json:"credentials,omitempty"`
}

type TLSFlags struct {
	Disabled           *bool   `json:"disabled,omitempty"`
	CACertsPath        *string `json:"caCertsPath,omitempty"`
	ServerName         *string `json:"serverName,omitempty"`
	CertFile           *string `json:"certFile,omitempty"`
	KeyFile            *string `json:"keyFile,omitempty"`
	InsecureSkipVerify *bool   `json:"insecureSkipVerify,omitempty"`
}

type CredentialsFlags struct {
	Type   *string                `json:"type,omitempty"`
	Static StaticCredentialsFlags `json:"static,omitempty"`
	Login  LoginCredentialsFlags  `json:"login,omitempty"`
}

type StaticCredentialsFlags struct {
	Token *string `json:"token,omitempty"`
}

type LoginCredentialsFlags struct {
	AuthMethod      *string           `json:"authMethod,omitempty"`
	Namespace       *string           `json:"namespace,omitempty"`
	Partition       *string           `json:"partition,omitempty"`
	Datacenter      *string           `json:"datacenter,omitempty"`
	BearerToken     *string           `json:"bearerToken,omitempty"`
	BearerTokenPath *string           `json:"bearerTokenPath,omitempty"`
	Meta            map[string]string `json:"meta,omitempty"`
}

type ServiceFlags struct {
	NodeName      *string `json:"nodeName,omitempty"`
	NodeID        *string `json:"nodeID,omitempty"`
	ServiceID     *string `json:"serviceID,omitempty"`
	ServiceIDPath *string `json:"serviceIDPath,omitempty"`
	Namespace     *string `json:"namespace,omitempty"`
	Partition     *string `json:"partition,omitempty"`
}

func (pf ProxyFlags) IsEmpty() bool {
	return pf.NodeName == nil &&
		pf.NodeID == nil &&
		pf.ID == nil &&
		pf.IDPath == nil &&
		pf.Namespace == nil &&
		pf.Partition == nil
}

type ProxyFlags struct {
	NodeName  *string `json:"nodeName,omitempty"`
	NodeID    *string `json:"nodeID,omitempty"`
	ID        *string `json:"id,omitempty"`
	IDPath    *string `json:"idPath,omitempty"`
	Namespace *string `json:"namespace,omitempty"`
	Partition *string `json:"partition,omitempty"`
}

type XDSServerFlags struct {
	BindAddr *string `json:"bindAddress,omitempty"`
	BindPort *int    `json:"bindPort,omitempty"`
}

type DNSServerFlags struct {
	BindAddr *string `json:"bindAddress,omitempty"`
	BindPort *int    `json:"bindPort,omitempty"`
}

type LogFlags struct {
	Name     string
	LogLevel *string `json:"logLevel,omitempty"`
	LogJSON  *bool   `json:"logJSON,omitempty"`
}

type TelemetryFlags struct {
	UseCentralConfig *bool                    `json:"useCentralConfig"`
	Prometheus       PrometheusTelemetryFlags `json:"prometheus,omitempty"`
}

type PrometheusTelemetryFlags struct {
	RetentionTime     *Duration `json:"retentionTime,omitempty"`
	CACertsPath       *string   `json:"caCertsPath,omitempty"`
	KeyFile           *string   `json:"keyFile,omitempty"`
	CertFile          *string   `json:"certFile,omitempty"`
	ServiceMetricsURL *string   `json:"serviceMetricsURL,omitempty"`
	ScrapePath        *string   `json:"scrapePath,omitempty"`
	MergePort         *int      `json:"mergePort,omitempty"`
}

type EnvoyFlags struct {
	AdminBindAddr    *string `json:"adminBindAddress,omitempty"`
	AdminBindPort    *int    `json:"adminBindPort,omitempty"`
	ReadyBindAddr    *string `json:"readyBindAddress,omitempty"`
	ReadyBindPort    *int    `json:"readyBindPort,omitempty"`
	Concurrency      *int    `json:"concurrency,omitempty"`
	DrainTimeSeconds *int    `json:"drainTimeSeconds,omitempty"`
	DrainStrategy    *string `json:"drainStrategy,omitempty"`

	ShutdownDrainListenersEnabled *bool   `json:"shutdownDrainListenersEnabled,omitempty"`
	ShutdownGracePeriodSeconds    *int    `json:"shutdownGracePeriodSeconds,omitempty"`
	GracefulShutdownPath          *string `json:"gracefulShutdownPath,omitempty"`
	GracefulAddr                  *string `json:"gracefulAddr,omitempty"`
	GracefulPort                  *int    `json:"gracefulPort,omitempty"`
	DumpEnvoyConfigOnExitEnabled  *bool   `json:"dumpEnvoyConfigOnExitEnabled,omitempty"`
	//Time in seconds to wait for dataplane to be ready.
	StartupGracePeriodSeconds *int `json:"startupGracePeriodSeconds,omitempty"`
	//Endpoint for graceful startup function.
	GracefulStartupPath *string `json:"gracefulStartupPath,omitempty"`
}

const (
	DefaultLogName = "consul-dataplane"
)

// buildDataplaneConfig builds the necessary config needed for the
// dataplane to start. We begin with the default version of the dataplane
// config(with the default values) followed by merging the file based
// config generated from the `-config-file` input into it.
// Since values given via CLI flags take the most precedence, we finally
// merge the config generated from the flags into the previously
// generated/merged config
func (f *FlagOpts) buildDataplaneConfig(extraArgs []string) (*consuldp.Config, error) {
	consulDPDefaultFlags, err := buildDefaultConsulDPFlags()
	if err != nil {
		return nil, err
	}

	if f.configFile != "" {
		consulDPFileBasedFlags, err := f.buildConfigFromFile()
		if err != nil {
			return nil, err
		}

		consulDPDefaultFlags, err = mergeConfigs(consulDPDefaultFlags, consulDPFileBasedFlags)
		if err != nil {
			return nil, err
		}
	}

	consulDPDefaultFlags, err = mergeConfigs(consulDPDefaultFlags, f.dataplaneConfig)
	if err != nil {
		return nil, err
	}

	consuldpRuntimeConfig, err := constructRuntimeConfig(consulDPDefaultFlags, extraArgs)
	if err != nil {
		return nil, err
	}

	return consuldpRuntimeConfig, nil
}

// Constructs a config based on the values present in the config json file
func (f *FlagOpts) buildConfigFromFile() (DataplaneConfigFlags, error) {
	var cfg DataplaneConfigFlags
	data, err := os.ReadFile(f.configFile)
	if err != nil {
		return DataplaneConfigFlags{}, err
	}

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return DataplaneConfigFlags{}, err
	}

	return cfg, nil
}

// Constructs a config with the default values
func buildDefaultConsulDPFlags() (DataplaneConfigFlags, error) {
	data := `
	{
		"mode": "sidecar",
		"consul": {
			"grpcPort": 8502,
			"serverWatchDisabled": false,
			"tls": {
				"disabled": false,
				"insecureSkipVerify": false
			}
		},
		"logging": {
			"name": "consul-dataplane",
			"logJSON": false,
			"logLevel": "info"
		},
		"telemetry": {
			"useCentralConfig": true,
			"prometheus": {
				"retentionTime": "60s",
				"scrapePath": "/metrics",
				"mergePort": 20100
			}
		},
		"envoy": {
			"adminBindAddress": "127.0.0.1",
			"adminBindPort": 19000,
			"readyBindPort": 0,
			"concurrency": 2,
			"drainTimeSeconds": 30,
			"drainStrategy": "immediate",
			"shutdownDrainListenersEnabled": false,
			"shutdownGracePeriodSeconds": 0,
			"gracefulShutdownPath": "/graceful_shutdown",
			"gracefulPort": 20300,
			"dumpEnvoyConfigOnExitEnabled": false,
			"gracefulStartupPath": "/graceful_startup",
			"startupGracePeriodSeconds": 0
		},
		"xdsServer": {
			"bindAddress": "127.0.0.1",
			"bindPort": 0
		},
		"dnsServer": {
			"bindAddress": "127.0.0.1",
			"bindPort": -1
		}
	}`

	var defaultCfgFlags DataplaneConfigFlags
	err := json.Unmarshal([]byte(data), &defaultCfgFlags)
	if err != nil {
		return DataplaneConfigFlags{}, err
	}

	return defaultCfgFlags, nil
}

// constructRuntimeConfig constructs the final config needed for dataplane to start
// itself after substituting all the user provided inputs
func constructRuntimeConfig(cfg DataplaneConfigFlags, extraArgs []string) (*consuldp.Config, error) {
	// Handle deprecated service flags.
	var proxyCfg consuldp.ProxyConfig
	if !cfg.Proxy.IsEmpty() {
		proxyCfg = consuldp.ProxyConfig{
			NodeName:  stringVal(cfg.Proxy.NodeName),
			NodeID:    stringVal(cfg.Proxy.NodeID),
			ProxyID:   stringVal(cfg.Proxy.ID),
			Namespace: stringVal(cfg.Proxy.Namespace),
			Partition: stringVal(cfg.Proxy.Partition),
		}
	} else {
		proxyCfg = consuldp.ProxyConfig{
			NodeName:  stringVal(cfg.Service.NodeName),
			NodeID:    stringVal(cfg.Service.NodeID),
			ProxyID:   stringVal(cfg.Service.ServiceID),
			Namespace: stringVal(cfg.Service.Namespace),
			Partition: stringVal(cfg.Service.Partition),
		}
	}

	return &consuldp.Config{
		Consul: &consuldp.ConsulConfig{
			Addresses:           stringVal(cfg.Consul.Addresses),
			GRPCPort:            intVal(cfg.Consul.GRPCPort),
			ServerWatchDisabled: boolVal(cfg.Consul.ServerWatchDisabled),
			Credentials: &consuldp.CredentialsConfig{
				Type: consuldp.CredentialsType(stringVal(cfg.Consul.Credentials.Type)),
				Static: consuldp.StaticCredentialsConfig{
					Token: stringVal(cfg.Consul.Credentials.Static.Token),
				},
				Login: consuldp.LoginCredentialsConfig{
					AuthMethod:      stringVal(cfg.Consul.Credentials.Login.AuthMethod),
					Namespace:       stringVal(cfg.Consul.Credentials.Login.Namespace),
					Partition:       stringVal(cfg.Consul.Credentials.Login.Partition),
					Datacenter:      stringVal(cfg.Consul.Credentials.Login.Datacenter),
					BearerToken:     stringVal(cfg.Consul.Credentials.Login.BearerToken),
					BearerTokenPath: stringVal(cfg.Consul.Credentials.Login.BearerTokenPath),
					Meta:            cfg.Consul.Credentials.Login.Meta,
				},
			},
			TLS: &consuldp.TLSConfig{
				Disabled:           boolVal(cfg.Consul.TLS.Disabled),
				CACertsPath:        stringVal(cfg.Consul.TLS.CACertsPath),
				CertFile:           stringVal(cfg.Consul.TLS.CertFile),
				KeyFile:            stringVal(cfg.Consul.TLS.KeyFile),
				ServerName:         stringVal(cfg.Consul.TLS.ServerName),
				InsecureSkipVerify: boolVal(cfg.Consul.TLS.InsecureSkipVerify),
			},
		},
		Mode:  consuldp.ModeType(stringVal(cfg.Mode)),
		Proxy: &proxyCfg,
		Logging: &consuldp.LoggingConfig{
			Name:     DefaultLogName,
			LogJSON:  boolVal(cfg.Logging.LogJSON),
			LogLevel: strings.ToUpper(stringVal(cfg.Logging.LogLevel)),
		},
		Envoy: &consuldp.EnvoyConfig{
			AdminBindAddress:              stringVal(cfg.Envoy.AdminBindAddr),
			AdminBindPort:                 intVal(cfg.Envoy.AdminBindPort),
			ReadyBindAddress:              stringVal(cfg.Envoy.ReadyBindAddr),
			ReadyBindPort:                 intVal(cfg.Envoy.ReadyBindPort),
			EnvoyConcurrency:              intVal(cfg.Envoy.Concurrency),
			EnvoyDrainTimeSeconds:         intVal(cfg.Envoy.DrainTimeSeconds),
			EnvoyDrainStrategy:            stringVal(cfg.Envoy.DrainStrategy),
			ShutdownDrainListenersEnabled: boolVal(cfg.Envoy.ShutdownDrainListenersEnabled),
			ShutdownGracePeriodSeconds:    intVal(cfg.Envoy.ShutdownGracePeriodSeconds),
			DumpEnvoyConfigOnExitEnabled:  boolVal(cfg.Envoy.DumpEnvoyConfigOnExitEnabled),
			GracefulShutdownPath:          stringVal(cfg.Envoy.GracefulShutdownPath),
			GracefulAddr:                  stringVal(cfg.Envoy.GracefulAddr),
			GracefulPort:                  intVal(cfg.Envoy.GracefulPort),
			StartupGracePeriodSeconds:     intVal(cfg.Envoy.StartupGracePeriodSeconds),
			GracefulStartupPath:           stringVal(cfg.Envoy.GracefulStartupPath),
			ExtraArgs:                     extraArgs,
		},
		Telemetry: &consuldp.TelemetryConfig{
			UseCentralConfig: boolVal(cfg.Telemetry.UseCentralConfig),
			Prometheus: consuldp.PrometheusTelemetryConfig{
				RetentionTime:     durationVal(cfg.Telemetry.Prometheus.RetentionTime),
				CACertsPath:       stringVal(cfg.Telemetry.Prometheus.CACertsPath),
				CertFile:          stringVal(cfg.Telemetry.Prometheus.CertFile),
				KeyFile:           stringVal(cfg.Telemetry.Prometheus.KeyFile),
				ServiceMetricsURL: stringVal(cfg.Telemetry.Prometheus.ServiceMetricsURL),
				ScrapePath:        stringVal(cfg.Telemetry.Prometheus.ScrapePath),
				MergePort:         intVal(cfg.Telemetry.Prometheus.MergePort),
			},
		},
		XDSServer: &consuldp.XDSServer{
			BindAddress: stringVal(cfg.XDSServer.BindAddr),
			BindPort:    intVal(cfg.XDSServer.BindPort),
		},
		DNSServer: &consuldp.DNSServerConfig{
			BindAddr: stringVal(cfg.DNSServer.BindAddr),
			Port:     intVal(cfg.DNSServer.BindPort),
		},
	}, nil
}

func mergeConfigs(c1, c2 DataplaneConfigFlags) (DataplaneConfigFlags, error) {
	err := mergo.Merge(&c1, c2, mergo.WithOverride, mergo.WithoutDereference)
	if err != nil {
		return DataplaneConfigFlags{}, err
	}

	return c1, nil
}
