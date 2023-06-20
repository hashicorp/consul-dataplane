package main

// mergeConfigs takes in a couple of configs(c1&c2) and applies
// c2 on to c1 based on the following conditions
//
// 1. If an attribute of c2 is a string/integer/bool, then it is applied to the
// same field in c1 if it is non nil indicating that the user provided the input
//  2. If an attribute of c2 is a slice/map and it is non nil then it overrides
//     the same field in c1
func mergeConfigs(c1, c2 *DataplaneConfigFlags) {
	c1.Consul = mergeConsulConfigs(c1.Consul, c2.Consul)
	c1.Service = mergeServiceConfigs(c1.Service, c2.Service)
	c1.Telemetry = mergeTelemetryConfigs(c1.Telemetry, c2.Telemetry)
	c1.Logging = mergeLoggingConfigs(c1.Logging, c2.Logging)
	c1.Envoy = mergeEnvoyConfigs(c1.Envoy, c2.Envoy)
	c1.XDSServer = mergeXDSServerConfigs(c1.XDSServer, c2.XDSServer)
	c1.DNSServer = mergeDNSServerConfigs(c1.DNSServer, c2.DNSServer)
}

func mergeConsulConfigs(c1, c2 ConsulFlags) ConsulFlags {
	if c2.Addresses != nil {
		c1.Addresses = c2.Addresses
	}

	if c2.GRPCPort != nil {
		c1.GRPCPort = c2.GRPCPort
	}

	if c2.ServerWatchDisabled != nil {
		c1.ServerWatchDisabled = c2.ServerWatchDisabled
	}

	if c2.Credentials.Type != nil {
		c1.Credentials.Type = c2.Credentials.Type
	}

	if c2.Credentials.Login.AuthMethod != nil {
		c1.Credentials.Login.AuthMethod = c2.Credentials.Login.AuthMethod
	}

	if c2.Credentials.Login.BearerToken != nil {
		c1.Credentials.Login.BearerToken = c2.Credentials.Login.BearerToken
	}

	if c2.Credentials.Login.BearerTokenPath != nil {
		c1.Credentials.Login.BearerTokenPath = c2.Credentials.Login.BearerTokenPath
	}

	if c2.Credentials.Login.Datacenter != nil {
		c1.Credentials.Login.Datacenter = c2.Credentials.Login.Datacenter
	}

	if c2.Credentials.Login.Namespace != nil {
		c1.Credentials.Login.Namespace = c2.Credentials.Login.Namespace
	}

	if c2.Credentials.Login.Partition != nil {
		c1.Credentials.Login.Partition = c2.Credentials.Login.Partition
	}

	if c2.Credentials.Login.Meta != nil {
		c1.Credentials.Login.Meta = c2.Credentials.Login.Meta
	}

	if c2.Credentials.Static.Token != nil {
		c1.Credentials.Static.Token = c2.Credentials.Static.Token
	}

	if c2.TLS.Disabled != nil {
		c1.TLS.Disabled = c2.TLS.Disabled
	}

	if c2.TLS.CACertsPath != nil {
		c1.TLS.CACertsPath = c2.TLS.CACertsPath
	}

	if c2.TLS.CertFile != nil {
		c1.TLS.CertFile = c2.TLS.CertFile
	}

	if c2.TLS.KeyFile != nil {
		c1.TLS.KeyFile = c2.TLS.KeyFile
	}

	if c2.TLS.ServerName != nil {
		c1.TLS.ServerName = c2.TLS.ServerName
	}

	if c2.TLS.InsecureSkipVerify != nil {
		c1.TLS.InsecureSkipVerify = c2.TLS.InsecureSkipVerify
	}

	return c1
}

func mergeTelemetryConfigs(c1, c2 TelemetryFlags) TelemetryFlags {
	if c2.UseCentralConfig != nil {
		c1.UseCentralConfig = c2.UseCentralConfig
	}

	if c2.Prometheus.RetentionTime != nil {
		c1.Prometheus.RetentionTime = c2.Prometheus.RetentionTime
	}

	if c2.Prometheus.CACertsPath != nil {
		c1.Prometheus.CACertsPath = c2.Prometheus.CACertsPath
	}

	if c2.Prometheus.CertFile != nil {
		c1.Prometheus.CertFile = c2.Prometheus.CertFile
	}

	if c2.Prometheus.KeyFile != nil {
		c1.Prometheus.KeyFile = c2.Prometheus.KeyFile
	}

	if c2.Prometheus.ScrapePath != nil {
		c1.Prometheus.ScrapePath = c2.Prometheus.ScrapePath
	}

	if c2.Prometheus.ServiceMetricsURL != nil {
		c1.Prometheus.ServiceMetricsURL = c2.Prometheus.ServiceMetricsURL
	}

	if c2.Prometheus.MergePort != nil {
		c1.Prometheus.MergePort = c2.Prometheus.MergePort
	}

	return c1
}

func mergeServiceConfigs(c1, c2 ServiceFlags) ServiceFlags {
	if c2.NodeName != nil {
		c1.NodeName = c2.NodeName
	}

	if c2.NodeID != nil {
		c1.NodeID = c2.NodeID
	}

	if c2.Namespace != nil {
		c1.Namespace = c2.Namespace
	}

	if c2.Partition != nil {
		c1.Partition = c2.Partition
	}

	if c2.ServiceID != nil {
		c1.ServiceID = c2.ServiceID
	}

	return c1
}

func mergeLoggingConfigs(c1, c2 LogFlags) LogFlags {
	if c2.LogJSON != nil {
		c1.LogJSON = c2.LogJSON
	}

	if c2.LogLevel != nil {
		c1.LogLevel = c2.LogLevel
	}

	return c1
}

func mergeEnvoyConfigs(c1, c2 EnvoyFlags) EnvoyFlags {
	if c2.AdminBindAddr != nil {
		c1.AdminBindAddr = c2.AdminBindAddr
	}

	if c2.AdminBindPort != nil {
		c1.AdminBindPort = c2.AdminBindPort
	}

	if c2.ReadyBindAddr != nil {
		c1.ReadyBindAddr = c2.ReadyBindAddr
	}

	if c2.ReadyBindPort != nil {
		c1.ReadyBindPort = c2.ReadyBindPort
	}

	if c2.Concurrency != nil {
		c1.Concurrency = c2.Concurrency
	}

	if c2.DrainTimeSeconds != nil {
		c1.DrainTimeSeconds = c2.DrainTimeSeconds
	}

	if c2.DrainStrategy != nil {
		c1.DrainStrategy = c2.DrainStrategy
	}

	if c2.ShutdownDrainListenersEnabled != nil {
		c1.ShutdownDrainListenersEnabled = c2.ShutdownDrainListenersEnabled
	}

	if c2.ShutdownGracePeriodSeconds != nil {
		c1.ShutdownGracePeriodSeconds = c2.ShutdownGracePeriodSeconds
	}

	if c2.GracefulShutdownPath != nil {
		c1.GracefulShutdownPath = c2.GracefulShutdownPath
	}

	if c2.GracefulPort != nil {
		c1.GracefulPort = c2.GracefulPort
	}

	if c2.DumpEnvoyConfigOnExitEnabled != nil {
		c1.DumpEnvoyConfigOnExitEnabled = c2.DumpEnvoyConfigOnExitEnabled
	}

	return c1
}

func mergeXDSServerConfigs(c1, c2 XDSServerFlags) XDSServerFlags {
	if c2.BindAddr != nil {
		c1.BindAddr = c2.BindAddr
	}

	if c2.BindPort != nil {
		c1.BindPort = c2.BindPort
	}

	return c1
}

func mergeDNSServerConfigs(c1, c2 DNSServerFlags) DNSServerFlags {
	if c2.BindAddr != nil {
		c1.BindAddr = c2.BindAddr
	}

	if c2.BindPort != nil {
		c1.BindPort = c2.BindPort
	}

	return c1
}
