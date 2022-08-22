package consuldp

// ConsulConfig are the settings required to connect with Consul servers
type ConsulConfig struct {
	// Addresses are Consul server addresses. Value can be:
	// DNS name OR 'exec=<executable with optional args>'.
	// Executable will be parsed by https://github.com/hashicorp/go-netaddrs.
	Addresses string
	// GRPCPort is the gRPC port on the Consul server.
	GRPCPort int
	// Credentials are the credentials used to authenticate requests and streams
	// to the Consul servers (e.g. static ACL token or auth method credentials).
	Credentials *CredentialsConfig
}

// CredentialsConfig contains the credentials used to authenticate requests and
// streams to the Consul servers.
type CredentialsConfig struct {
	// Static contains the static ACL token.
	Static *StaticCredentialsConfig
}

// StaticCredentialsConfig contains the static ACL token that will be used to
// authenticate requests and streams to the Consul servers.
type StaticCredentialsConfig struct {
	// Token is the static ACL token.
	Token string
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

// ServiceConfig contains details of the proxy service instance.
type ServiceConfig struct {
	// NodeName is the name of the node to which the proxy service instance is
	// registered.
	NodeName string
	// NodeName is the ID of the node to which the proxy service instance is
	// registered.
	NodeID string
	// ServiceID is the ID of the proxy service instance.
	ServiceID string
	// Namespace is the Consul Enterprise namespace in which the proxy service
	// instance is registered.
	Namespace string
	// Partition is the Consul Enterprise partition in which the proxy service
	// instance is registered.
	Partition string
}

// TelemetryConfig contains configuration for telemetry.
type TelemetryConfig struct {
	// UseCentralConfig controls whether the proxy will apply the central telemetry
	// configuration.
	UseCentralConfig bool

	// TODO(NET-??): Local telemetry configuration.
}

// EnvoyConfig contains configuration for the Envoy process.
type EnvoyConfig struct {
	// AdminBindAddress is the address on which the Envoy admin server will be available.
	AdminBindAddress string
	// AdminBindPort is the port on which the Envoy admin server will be available.
	AdminBindPort int
	// ReadyBindAddress is the address on which the Envoy readiness probe will be available.
	ReadyBindAddress string
	// ReadyBindPort is the port on which the Envoy readiness probe will be available.
	ReadyBindPort int
}

// Config is the configuration used by consul-dataplane, consolidated
// from various sources - CLI flags, env vars, config file settings.
type Config struct {
	Consul    *ConsulConfig
	Service   *ServiceConfig
	Logging   *LoggingConfig
	Telemetry *TelemetryConfig
	Envoy     *EnvoyConfig
}
