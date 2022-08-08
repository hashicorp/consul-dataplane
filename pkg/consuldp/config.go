package consuldp

// ConsulConfig are the settings required to connect with Consul servers
type ConsulConfig struct {
	// Addresses are Consul server addresses. Value can be:
	// DNS name OR 'exec=<executable with optional args>'.
	// Executable will be parsed by https://github.com/hashicorp/go-netaddrs.
	Addresses string
	// GRPCPort is the gRPC port on the Consul server.
	GRPCPort int
}

// LoggingConfig can be used to specify logger configuration settings.
type LoggingConfig struct {
	// Name of the subsystem to prefix logs with
	Name string
	// LogLevel is the logging level. Valid values - TRACE, DEBUG, INFO, WARN, ERROR
	LogLevel string
	// LogJSON controls if the output should be in JSON.
	LogJSON bool
}

// RuntimeConfig is the configuration used by consul-dataplane, consolidated
// from various sources - CLI flags, env vars, config file settings.
type RuntimeConfig struct {
	Consul *ConsulConfig

	Logging *LoggingConfig
}
