// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"crypto/tls"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/consul-server-connection-manager/discovery"
	"github.com/hashicorp/go-rootcerts"
)

// ConsulConfig are the settings required to connect with Consul servers
type ConsulConfig struct {
	// Addresses are Consul server addresses. Value can be:
	// DNS name OR 'exec=<executable with optional args>'.
	// Executable will be parsed by https://github.com/hashicorp/go-netaddrs.
	Addresses string `json:"addresses"`
	// GRPCPort is the gRPC port on the Consul server.
	GRPCPort int `json:"grpcPort"`
	// Credentials are the credentials used to authenticate requests and streams
	// to the Consul servers (e.g. static ACL token or auth method credentials).
	Credentials *CredentialsConfig `json:"credentials,omitempty"`
	// ServerWatchDisabled opts-out of consuming the server update stream, for
	// cases where its addresses are incorrect (e.g. servers are behind a load
	// balancer).
	ServerWatchDisabled bool `json:"serverWatchDisabled"`
	// TLS contains the TLS settings for communicating with Consul servers.
	TLS *TLSConfig `json:"tls,omitempty"`
}

// DNSServerConfig is the configuration for the transparent DNS proxy that will forward requests to consul
type DNSServerConfig struct {
	// BindAddr is the address the DNS server will bind to. Default will be 127.0.0.1
	BindAddr string `json:"bindAddr"`
	// Port is the port which the DNS server will bind to.
	Port int `json:"port"`
}

// TLSConfig contains the TLS settings for communicating with Consul servers.
type TLSConfig struct {
	// Disabled causes consul-dataplane to communicate with Consul servers over
	// an insecure plaintext connection. This is useful for testing, but should
	// not be used in production.
	Disabled bool `json:"disabled"`
	// CACertsPath is a path to a file or directory containing CA certificates to
	// use to verify the server's certificate. This is only necessary if the server
	// presents a certificate that isn't signed by a trusted public CA.
	CACertsPath string `json:"caCertsPath"`
	// ServerName is used to verify the server certificate's subject when it cannot
	// be inferred from Consul.Addresses (i.e. it is not a DNS name).
	ServerName string `json:"serverName"`
	// CertFile is a path to the client certificate that will be presented to
	// Consul servers.
	//
	// Note: this is only required if servers have tls.grpc.verify_incoming enabled.
	// Generally, issuing consul-dataplane instances with client certificates isn't
	// necessary and creates significant operational burden.
	CertFile string `json:"certFile"`
	// KeyFile is a path to the client private key that will be used to communicate
	// with Consul servers (when CertFile is provided).
	//
	// Note: this is only required if servers have tls.grpc.verify_incoming enabled.
	// Generally, issuing consul-dataplane instances with client certificates isn't
	// necessary and creates significant operational burden.
	KeyFile string `json:"keyFile"`
	// InsecureSkipVerify causes consul-dataplane not to verify the certificate
	// presented by the server. This is useful for testing, but should not be used
	// in production.
	InsecureSkipVerify bool `json:"insecureSkipVerify"`
}

// Load creates a *tls.Config, including loading the CA and client certificates.
func (t *TLSConfig) Load() (*tls.Config, error) {
	if t.Disabled {
		return nil, nil
	}

	tlsCfg := &tls.Config{
		ServerName:         t.ServerName,
		InsecureSkipVerify: t.InsecureSkipVerify,
	}

	var rootCfg rootcerts.Config
	if path := t.CACertsPath; path != "" {
		fi, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certs: %w", err)
		}
		if fi.IsDir() {
			rootCfg.CAPath = path
		} else {
			rootCfg.CAFile = path
		}
	}
	if err := rootcerts.ConfigureTLS(tlsCfg, &rootCfg); err != nil {
		return nil, fmt.Errorf("failed to configure CA certs: %w", err)
	}

	if t.CertFile != "" && t.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to configure TLS cert: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}

// CredentialsConfig contains the credentials used to authenticate requests and
// streams to the Consul servers.
type CredentialsConfig struct {
	// Type identifies the type of credentials provided.
	Type CredentialsType `json:"type"`
	// Static contains the static ACL token.
	Static StaticCredentialsConfig `json:"static"`
	// Login contains the credentials for logging in with an auth method.
	Login LoginCredentialsConfig `json:"login"`
}

// CredentialsType identifies the type of credentials provided.
type CredentialsType string

const (
	// CredentialsTypeNone indicates that no credentials were given.
	CredentialsTypeNone CredentialsType = ""
	// CredentialsTypeStatic indicates that a static ACL token was provided.
	CredentialsTypeStatic CredentialsType = "static"
	// CredentialsTypeLogin indicates that credentials were provided to log in with
	// an auth method.
	CredentialsTypeLogin CredentialsType = "login"
)

// StaticCredentialsConfig contains the static ACL token that will be used to
// authenticate requests and streams to the Consul servers.
type StaticCredentialsConfig struct {
	// Token is the static ACL token.
	Token string `json:"token"`
}

// LoginCredentialsConfig contains credentials for logging in with an auth method.
type LoginCredentialsConfig struct {
	// AuthMethod is the name of the Consul auth method.
	AuthMethod string `json:"authmethod"`
	// Namespace is the namespace containing the auth method.
	Namespace string `json:"namespace"`
	// Partition is the partition containing the auth method.
	Partition string `json:"partition"`
	// Datacenter is the datacenter containing the auth method.
	Datacenter string `json:"datacenter"`
	// BearerToken is the bearer token presented to the auth method.
	BearerToken string `json:"bearerToken"`
	// BearerTokenPath is the path to a file containing a bearer token.
	BearerTokenPath string `json:"bearerTokenPath"`
	// Meta is the arbitrary set of key-value pairs to attach to the
	// token. These are included in the Description field of the token.
	Meta map[string]string `json:"meta,omitempty"`
}

// ToDiscoveryCredentials creates a discovery.Credentials, including loading a
// bearer token from a file if BearerPath is given.
func (cc *CredentialsConfig) ToDiscoveryCredentials() (discovery.Credentials, error) {
	var creds discovery.Credentials

	switch cc.Type {
	case CredentialsTypeNone:
		return creds, nil
	case CredentialsTypeStatic:
		creds.Type = discovery.CredentialsTypeStatic
		creds.Static = discovery.StaticTokenCredential{
			Token: cc.Static.Token,
		}
	case CredentialsTypeLogin:
		creds.Type = discovery.CredentialsTypeLogin
		creds.Login = discovery.LoginCredential{
			AuthMethod:  cc.Login.AuthMethod,
			Namespace:   cc.Login.Namespace,
			Partition:   cc.Login.Partition,
			Datacenter:  cc.Login.Datacenter,
			BearerToken: cc.Login.BearerToken,
			Meta:        cc.Login.Meta,
		}

		if creds.Login.BearerToken == "" && cc.Login.BearerTokenPath != "" {
			bearer, err := os.ReadFile(cc.Login.BearerTokenPath)
			if err != nil {
				return creds, fmt.Errorf("failed to read bearer token from file: %w", err)
			}
			creds.Login.BearerToken = string(bearer)
		}
	default:
		return creds, fmt.Errorf("unknown credential type: %s", cc.Type)
	}

	return creds, nil
}

// LoggingConfig can be used to specify logger configuration settings.
type LoggingConfig struct {
	// Name of the subsystem to prefix logs with
	Name string
	// LogLevel is the logging level. Valid values - TRACE, DEBUG, INFO, WARN, ERROR
	LogLevel string `json:"logLevel"`
	// LogJSON controls if the output should be in JSON.
	LogJSON bool `json:"logJSON"`
}

// ServiceConfig contains details of the proxy service instance.
type ServiceConfig struct {
	// NodeName is the name of the node to which the proxy service instance is
	// registered.
	NodeName string `json:"nodeName"`
	// NodeName is the ID of the node to which the proxy service instance is
	// registered.
	NodeID string `json:"nodeId"`
	// ServiceID is the ID of the proxy service instance.
	ServiceID string `json:"serviceId"`
	// Namespace is the Consul Enterprise namespace in which the proxy service
	// instance is registered.
	Namespace string `json:"namespace"`
	// Partition is the Consul Enterprise partition in which the proxy service
	// instance is registered.
	Partition string `json:"partition"`
}

// TelemetryConfig contains configuration for telemetry.
type TelemetryConfig struct {
	// UseCentralConfig controls whether the proxy will apply the central telemetry
	// configuration.
	UseCentralConfig bool `json:"useCentralConfig"`
	// Prometheus contains Prometheus-specific configuration that cannot be
	// determined from central telemetry configuration.
	Prometheus PrometheusTelemetryConfig `json:"prometheus"`
}

// PrometheusTelemetryConfig contains Prometheus-specific telemetry config.
type PrometheusTelemetryConfig struct {
	// RetentionTime controls the duration that metrics are aggregated for.
	RetentionTime time.Duration `json:"retentionTime"`
	// CACertsPath is a path to a file or directory containing CA certificates
	// to use to verify the Prometheus server's certificate. This is only
	// necessary if the server presents a certificate that isn't signed by a
	// trusted public CA.
	CACertsPath string `json:"caCertsPath"`
	// KeyFile is a path to the client private key used for serving Prometheus
	// metrics.
	KeyFile string `json:"keyFile"`
	// CertFile is a path to the client certificate used for serving Prometheus
	// metrics.
	CertFile string `json:"certFile"`
	// ServiceMetricsURL is an optional URL that must serve Prometheus metrics.
	// The metrics at this URL are scraped and merged into Consul Dataplane's
	// main Prometheus metrics.
	ServiceMetricsURL string `json:"serviceMetricsURL"`
	// ScrapePath is the URL path where Envoy serves Prometheus metrics.
	ScrapePath string `json:"scrapePath"`
	// MergePort is the port to server merged metrics.
	MergePort int `json:"mergePort"`
}

// EnvoyConfig contains configuration for the Envoy process.
type EnvoyConfig struct {
	// AdminBindAddress is the address on which the Envoy admin server will be available.
	AdminBindAddress string `json:"adminBindAddress"`
	// AdminBindPort is the port on which the Envoy admin server will be available.
	AdminBindPort int `json:"adminBindPort"`
	// ReadyBindAddress is the address on which the Envoy readiness probe will be available.
	ReadyBindAddress string `json:"readyBindAddress"`
	// ReadyBindPort is the port on which the Envoy readiness probe will be available.
	ReadyBindPort int `json:"readyBindPort"`
	// EnvoyConcurrency is the envoy concurrency https://www.envoyproxy.io/docs/envoy/latest/operations/cli#cmdoption-concurrency
	EnvoyConcurrency int `json:"envoyConcurrency"`
	// EnvoyDrainTime is the time in seconds for which Envoy will drain connections
	// during a hot restart, when listeners are modified or removed via LDS, or when
	// initiated manually via a request to the Envoy admin API.
	// The Envoy HTTP connection manager filter will add “Connection: close” to HTTP1
	// requests, send HTTP2 GOAWAY, and terminate connections on request completion
	// (after the delayed close period).
	// https://www.envoyproxy.io/docs/envoy/latest/operations/cli#cmdoption-drain-time-s
	EnvoyDrainTimeSeconds int `json:"envoyDrainTimeSeconds"`
	// EnvoyDrainStrategy is the behaviour of Envoy during the drain sequence.
	// Determines whether all open connections should be encouraged to drain
	// immediately or to increase the percentage gradually as the drain time elapses.
	// https://www.envoyproxy.io/docs/envoy/latest/operations/cli#cmdoption-drain-strategy
	EnvoyDrainStrategy string `json:"envoyDrainStrategy"`
	// ShutdownDrainListenersEnabled configures whether to start draining proxy listeners before terminating the proxy container. Drain time defaults to the value of ShutdownGracePeriodSeconds, but may be set explicitly with EnvoyDrainTimeSeconds.
	ShutdownDrainListenersEnabled bool `json:"shutdownDrainListenersEnabled"`
	// ShutdownGracePeriodSeconds is the amount of time to wait after receiving a SIGTERM before terminating the proxy container.
	ShutdownGracePeriodSeconds int `json:"shutdownGracePeriodSeconds"`
	// GracefulShutdownPath is the path on which the HTTP endpoint to initiate a graceful shutdown of Envoy is served
	GracefulShutdownPath string `json:"gracefulShutdownPath"`
	// GracefulPort is the port on which the HTTP server for graceful shutdown endpoints will be available.
	GracefulPort int `json:"gracefulPort"`
	// DumpEnvoyConfigOnExitEnabled configures whether to call Envoy's /config_dump endpoint during consul-dataplane controlled shutdown.
	DumpEnvoyConfigOnExitEnabled bool `json:"dumpEnvoyConfigOnExitEnabled"`
	// ExtraArgs are the extra arguments passed to envoy at startup of the proxy
	ExtraArgs []string `json:"extraArgs,omitempty"`
}

// XDSServer contains the configuration of the xDS server.
type XDSServer struct {
	// BindAddress is the address on which the Envoy xDS server will be available.
	BindAddress string `json:"bindAddress"`
	// BindPort is the address on which the Envoy xDS port will be available.
	BindPort int `json:"bindPort"`
}

// Config is the configuration used by consul-dataplane, consolidated
// from various sources - CLI flags, env vars, config file settings.
type Config struct {
	DNSServer *DNSServerConfig `json:"dnsServerConfig,omitempty"`
	Consul    *ConsulConfig    `json:"consul,omitempty"`
	Service   *ServiceConfig   `json:"service,omitempty"`
	Logging   *LoggingConfig   `json:"logging,omitempty"`
	Telemetry *TelemetryConfig `json:"telemetry,omitempty"`
	Envoy     *EnvoyConfig     `json:"envoy,omitempty"`
	XDSServer *XDSServer       `json:"xdsServer,omitempty"`
}
