package main

import (
	"strings"

	"github.com/hashicorp/consul-dataplane/pkg/consuldp"
)

// mergeConfigs takes in a couple of configs(c1&c2) and applies
// c2 on to c1 based on the following conditions
//
// 1. If an attribute of c2 is a string/integer/bool, then it is applied to the
// same field in c1 if it holds non default value for the field
// 2. If an attribute of c2 is a slice/map, then it overrides the same field in c1
func mergeConfigs(c1, c2 *consuldp.Config) {
	mergeConsulConfigs(c1.Consul, c2.Consul)
	mergeServiceConfigs(c1.Service, c2.Service)
	mergeTelemetryConfigs(c1.Telemetry, c2.Telemetry)
	mergeLoggingConfigs(c1.Logging, c2.Logging)
	mergeEnvoyConfigs(c1.Envoy, c2.Envoy)
	mergeXDSServerConfigs(c1.XDSServer, c2.XDSServer)
	mergeDNSServerConfigs(c1.DNSServer, c2.DNSServer)
}

func mergeConsulConfigs(c1, c2 *consuldp.ConsulConfig) {
	if c2 == nil {
		return
	}

	if c2.Addresses != "" {
		c1.Addresses = c2.Addresses
	}

	if c2.GRPCPort != int(0) && c2.GRPCPort != DefaultGRPCPort {
		c1.GRPCPort = c2.GRPCPort
	}

	if c2.ServerWatchDisabled {
		c1.ServerWatchDisabled = true
	}

	if c2.Credentials != nil {
		if c2.Credentials.Type != "" {
			c1.Credentials.Type = c2.Credentials.Type
		}

		if c2.Credentials.Login.AuthMethod != "" {
			c1.Credentials.Login.AuthMethod = c2.Credentials.Login.AuthMethod
		}

		if c2.Credentials.Login.BearerToken != "" {
			c1.Credentials.Login.BearerToken = c2.Credentials.Login.BearerToken
		}

		if c2.Credentials.Login.BearerTokenPath != "" {
			c1.Credentials.Login.BearerTokenPath = c2.Credentials.Login.BearerTokenPath
		}

		if c2.Credentials.Login.Datacenter != "" {
			c1.Credentials.Login.Datacenter = c2.Credentials.Login.Datacenter
		}

		if c2.Credentials.Login.Namespace != "" {
			c1.Credentials.Login.Namespace = c2.Credentials.Login.Namespace
		}

		if c2.Credentials.Login.Partition != "" {
			c1.Credentials.Login.Partition = c2.Credentials.Login.Partition
		}

		if c2.Credentials.Login.Meta != nil {
			c1.Credentials.Login.Meta = c2.Credentials.Login.Meta
		}

		if c2.Credentials.Static.Token != "" {
			c1.Credentials.Static.Token = c2.Credentials.Static.Token
		}
	}

	if c2.TLS != nil {
		if c2.TLS.Disabled {
			c1.TLS.Disabled = true
		}

		if c2.TLS.CACertsPath != "" {
			c1.TLS.CACertsPath = c2.TLS.CACertsPath
		}

		if c2.TLS.CertFile != "" {
			c1.TLS.CertFile = c2.TLS.CertFile
		}

		if c2.TLS.KeyFile != "" {
			c1.TLS.KeyFile = c2.TLS.KeyFile
		}

		if c2.TLS.ServerName != "" {
			c1.TLS.ServerName = c2.TLS.ServerName
		}

		if c2.TLS.InsecureSkipVerify {
			c1.TLS.InsecureSkipVerify = true
		}
	}
}

func mergeTelemetryConfigs(c1, c2 *consuldp.TelemetryConfig) {
	if c2 == nil {
		return
	}

	if !c2.UseCentralConfig {
		c1.UseCentralConfig = false
	}

	if c2.Prometheus.RetentionTime != 0 && c2.Prometheus.RetentionTime != DefaultPromRetentionTime {
		c1.Prometheus.RetentionTime = c2.Prometheus.RetentionTime
	}

	if c2.Prometheus.CACertsPath != "" {
		c1.Prometheus.CACertsPath = c2.Prometheus.CACertsPath
	}

	if c2.Prometheus.CertFile != "" {
		c1.Prometheus.CertFile = c2.Prometheus.CertFile
	}

	if c2.Prometheus.KeyFile != "" {
		c1.Prometheus.KeyFile = c2.Prometheus.KeyFile
	}

	if c2.Prometheus.ScrapePath != "" {
		c1.Prometheus.ScrapePath = c2.Prometheus.ScrapePath
	}

	if c2.Prometheus.ServiceMetricsURL != "" {
		c1.Prometheus.ServiceMetricsURL = c2.Prometheus.ServiceMetricsURL
	}

	if c2.Prometheus.MergePort != int(0) && c2.Prometheus.MergePort != DefaultPromMergePort {
		c1.Prometheus.MergePort = c2.Prometheus.MergePort
	}
}

func mergeServiceConfigs(c1, c2 *consuldp.ServiceConfig) {
	if c2 == nil {
		return
	}

	if c2.NodeName != "" {
		c1.NodeName = c2.NodeName
	}

	if c2.NodeID != "" {
		c1.NodeID = c2.NodeID
	}

	if c2.Namespace != "" {
		c1.Namespace = c2.Namespace
	}

	if c2.Partition != "" {
		c1.Partition = c2.Partition
	}

	if c2.ServiceID != "" {
		c1.ServiceID = c2.ServiceID
	}
}

func mergeLoggingConfigs(c1, c2 *consuldp.LoggingConfig) {
	if c2 == nil {
		return
	}

	if c2.LogJSON {
		c1.LogJSON = true
	}

	if c2.LogLevel != "" && c2.LogLevel != DefaultLogLevel {
		c1.LogLevel = strings.ToUpper(c2.LogLevel)
	}
}

func mergeEnvoyConfigs(c1, c2 *consuldp.EnvoyConfig) {
	if c2 == nil {
		return
	}

	if c2.AdminBindAddress != "" && c2.AdminBindAddress != DefaultEnvoyAdminBindAddr {
		c1.AdminBindAddress = c2.AdminBindAddress
	}

	if c2.AdminBindPort != int(0) && c2.AdminBindPort != DefaultEnvoyAdminBindPort {
		c1.AdminBindPort = c2.AdminBindPort
	}

	if c2.ReadyBindAddress != "" {
		c1.ReadyBindAddress = c2.ReadyBindAddress
	}

	if c2.ReadyBindPort != int(0) && c2.ReadyBindPort != DefaultEnvoyReadyBindPort {
		c1.ReadyBindPort = c2.ReadyBindPort
	}

	if c2.EnvoyConcurrency != int(0) && c2.EnvoyConcurrency != DefaultEnvoyConcurrency {
		c1.EnvoyConcurrency = c2.EnvoyConcurrency
	}

	if c2.EnvoyDrainTimeSeconds != int(0) && c2.EnvoyDrainTimeSeconds != DefaultEnvoyDrainTimeSeconds {
		c1.EnvoyDrainTimeSeconds = c2.EnvoyDrainTimeSeconds
	}

	if c2.EnvoyDrainStrategy != "" && c2.EnvoyDrainStrategy != DefaultEnvoyDrainStrategy {
		c1.EnvoyDrainStrategy = c2.EnvoyDrainStrategy
	}

	if c2.ShutdownDrainListenersEnabled {
		c1.ShutdownDrainListenersEnabled = true
	}

	if c2.ShutdownGracePeriodSeconds != int(0) && c2.ShutdownGracePeriodSeconds != DefaultEnvoyShutdownGracePeriodSeconds {
		c1.ShutdownGracePeriodSeconds = c2.ShutdownGracePeriodSeconds
	}

	if c2.GracefulShutdownPath != "" && c2.GracefulShutdownPath != DefaultGracefulShutdownPath {
		c1.GracefulShutdownPath = c2.GracefulShutdownPath
	}

	if c2.GracefulPort != int(0) && c2.GracefulPort != DefaultGracefulPort {
		c1.GracefulPort = c2.GracefulPort
	}

	if c2.DumpEnvoyConfigOnExitEnabled {
		c1.DumpEnvoyConfigOnExitEnabled = true
	}

	if c2.ExtraArgs != nil && len(c2.ExtraArgs) > 0 {
		c1.ExtraArgs = append(c1.ExtraArgs, c2.ExtraArgs...)
	}
}

func mergeXDSServerConfigs(c1, c2 *consuldp.XDSServer) {
	if c2 == nil {
		return
	}

	if c2.BindAddress != "" && c2.BindAddress != DefaultXDSBindAddr {
		c1.BindAddress = c2.BindAddress
	}

	if c2.BindPort != int(0) && c2.BindPort != DefaultXDSBindPort {
		c1.BindPort = c2.BindPort
	}
}

func mergeDNSServerConfigs(c1, c2 *consuldp.DNSServerConfig) {
	if c2 == nil {
		return
	}

	if c2.BindAddr != "" && c2.BindAddr != DefaultDNSBindAddr {
		c1.BindAddr = c2.BindAddr
	}

	if c2.Port != int(0) && c2.Port != DefaultDNSBindPort {
		c1.Port = c2.Port
	}
}
