package main

import (
	"encoding/json"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/consul-dataplane/pkg/consuldp"
)

type FlagOpts struct {
	printVersion bool

	addresses           string
	grpcPort            int
	serverWatchDisabled bool

	tlsDisabled           bool
	tlsCACertsPath        string
	tlsServerName         string
	tlsCertFile           string
	tlsKeyFile            string
	tlsInsecureSkipVerify bool

	logLevel string
	logJSON  bool

	nodeName      string
	nodeID        string
	serviceID     string
	serviceIDPath string
	namespace     string
	partition     string

	credentialType       string
	token                string
	loginAuthMethod      string
	loginNamespace       string
	loginPartition       string
	loginDatacenter      string
	loginBearerToken     string
	loginBearerTokenPath string
	loginMeta            map[string]string

	useCentralTelemetryConfig bool

	promRetentionTime     time.Duration
	promCACertsPath       string
	promKeyFile           string
	promCertFile          string
	promServiceMetricsURL string
	promScrapePath        string
	promMergePort         int

	adminBindAddr         string
	adminBindPort         int
	readyBindAddr         string
	readyBindPort         int
	envoyConcurrency      int
	envoyDrainTimeSeconds int
	envoyDrainStrategy    string

	xdsBindAddr string
	xdsBindPort int

	consulDNSBindAddr string
	consulDNSPort     int

	shutdownDrainListenersEnabled bool
	shutdownGracePeriodSeconds    int
	gracefulShutdownPath          string
	gracefulPort                  int

	dumpEnvoyConfigOnExitEnabled bool

	configFile string
}

const (
	DefaultGRPCPort            = 8502
	DefaultServerWatchDisabled = false

	DefaultTLSDisabled           = false
	DefaultTLSInsecureSkipVerify = false

	DefaultDNSBindAddr = "127.0.0.1"
	DefaultDNSBindPort = -1

	DefaultXDSBindAddr = "127.0.0.1"
	DefaultXDSBindPort = 0

	DefaultLogLevel = "info"
	DefaultLogJSON  = false

	DefaultEnvoyAdminBindAddr                 = "127.0.0.1"
	DefaultEnvoyAdminBindPort                 = 19000
	DefaultEnvoyReadyBindPort                 = 0
	DefaultEnvoyConcurrency                   = 2
	DefaultEnvoyDrainTimeSeconds              = 30
	DefaultEnvoyDrainStrategy                 = "immediate"
	DefaultEnvoyShutdownDrainListenersEnabled = false
	DefaultEnvoyShutdownGracePeriodSeconds    = 0
	DefaultGracefulShutdownPath               = "/graceful_shutdown"
	DefaultGracefulPort                       = 20300
	DefaultDumpEnvoyConfigOnExitEnabled       = false

	DefaultUseCentralTelemetryConfig = true
	DefaultPromRetentionTime         = 60 * time.Second
	DefaultPromScrapePath            = "/metrics"
	DefaultPromMergePort             = 20100
)

// buildDataplaneConfig builds the necessary config needed for the
// dataplane to start. We begin with the default version of the dataplane
// config(with the default values) followed by merging the file based
// config generated from the `-config-file` input into it.
// Since values given via CLI flags take the most precedence, we finally
// merge the config generated from the flags into the previously
// generated/merged config
func (f *FlagOpts) buildDataplaneConfig() (*consuldp.Config, error) {
	var consuldpConfig, consuldpCfgFromFlags *consuldp.Config

	consuldpConfig = buildDefaultConsulDPConfig()
	consuldpCfgFromFlags = f.buildConfig()

	if f.configFile != "" {
		consuldpCfgFromFile, err := f.buildConfigFromFile()
		if err != nil {
			return nil, err
		}

		mergeConfigs(consuldpConfig, consuldpCfgFromFile)
	}

	mergeConfigs(consuldpConfig, consuldpCfgFromFlags)

	return consuldpConfig, nil
}

// Constructs a config based on the values present in the config json file
func (f *FlagOpts) buildConfigFromFile() (*consuldp.Config, error) {
	var cfg *consuldp.Config
	data, err := os.ReadFile(f.configFile)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// Constructs a config based on the values given via the CLI flags
func (f *FlagOpts) buildConfig() *consuldp.Config {
	return &consuldp.Config{
		Consul: &consuldp.ConsulConfig{
			Addresses: f.addresses,
			GRPCPort:  f.grpcPort,
			Credentials: &consuldp.CredentialsConfig{
				Type: consuldp.CredentialsType(f.credentialType),
				Static: consuldp.StaticCredentialsConfig{
					Token: f.token,
				},
				Login: consuldp.LoginCredentialsConfig{
					AuthMethod:      f.loginAuthMethod,
					Namespace:       f.loginNamespace,
					Partition:       f.loginPartition,
					Datacenter:      f.loginDatacenter,
					BearerToken:     f.loginBearerToken,
					BearerTokenPath: f.loginBearerTokenPath,
					Meta:            f.loginMeta,
				},
			},
			ServerWatchDisabled: f.serverWatchDisabled,
			TLS: &consuldp.TLSConfig{
				Disabled:           f.tlsDisabled,
				CACertsPath:        f.tlsCACertsPath,
				ServerName:         f.tlsServerName,
				CertFile:           f.tlsCertFile,
				KeyFile:            f.tlsKeyFile,
				InsecureSkipVerify: f.tlsInsecureSkipVerify,
			},
		},
		Service: &consuldp.ServiceConfig{
			NodeName:  f.nodeName,
			NodeID:    f.nodeID,
			ServiceID: f.serviceID,
			Namespace: f.namespace,
			Partition: f.partition,
		},
		Logging: &consuldp.LoggingConfig{
			Name:     "consul-dataplane",
			LogLevel: strings.ToUpper(f.logLevel),
			LogJSON:  f.logJSON,
		},
		Telemetry: &consuldp.TelemetryConfig{
			UseCentralConfig: f.useCentralTelemetryConfig,
			Prometheus: consuldp.PrometheusTelemetryConfig{
				RetentionTime:     f.promRetentionTime,
				CACertsPath:       f.promCACertsPath,
				KeyFile:           f.promKeyFile,
				CertFile:          f.promCertFile,
				ServiceMetricsURL: f.promServiceMetricsURL,
				ScrapePath:        f.promScrapePath,
				MergePort:         f.promMergePort,
			},
		},
		Envoy: &consuldp.EnvoyConfig{
			AdminBindAddress:              f.adminBindAddr,
			AdminBindPort:                 f.adminBindPort,
			ReadyBindAddress:              f.readyBindAddr,
			ReadyBindPort:                 f.readyBindPort,
			EnvoyConcurrency:              f.envoyConcurrency,
			EnvoyDrainTimeSeconds:         f.envoyDrainTimeSeconds,
			EnvoyDrainStrategy:            f.envoyDrainStrategy,
			ShutdownDrainListenersEnabled: f.shutdownDrainListenersEnabled,
			ShutdownGracePeriodSeconds:    f.shutdownGracePeriodSeconds,
			GracefulShutdownPath:          f.gracefulShutdownPath,
			GracefulPort:                  f.gracefulPort,
			DumpEnvoyConfigOnExitEnabled:  f.dumpEnvoyConfigOnExitEnabled,
			ExtraArgs:                     flag.Args(),
		},
		XDSServer: &consuldp.XDSServer{
			BindAddress: f.xdsBindAddr,
			BindPort:    f.xdsBindPort,
		},
		DNSServer: &consuldp.DNSServerConfig{
			BindAddr: f.consulDNSBindAddr,
			Port:     f.consulDNSPort,
		},
	}
}

// Constructs a config with the default values
func buildDefaultConsulDPConfig() *consuldp.Config {
	return &consuldp.Config{
		Consul: &consuldp.ConsulConfig{
			GRPCPort: DefaultGRPCPort,
			Credentials: &consuldp.CredentialsConfig{
				Type:   consuldp.CredentialsType(""),
				Static: consuldp.StaticCredentialsConfig{},
				Login: consuldp.LoginCredentialsConfig{
					Meta: map[string]string{},
				},
			},
			ServerWatchDisabled: DefaultServerWatchDisabled,
			TLS: &consuldp.TLSConfig{
				Disabled:           DefaultTLSDisabled,
				InsecureSkipVerify: DefaultTLSInsecureSkipVerify,
			},
		},
		Service: &consuldp.ServiceConfig{},
		Logging: &consuldp.LoggingConfig{
			Name:     "consul-dataplane",
			LogLevel: strings.ToUpper(DefaultLogLevel),
			LogJSON:  DefaultLogJSON,
		},
		Telemetry: &consuldp.TelemetryConfig{
			UseCentralConfig: DefaultUseCentralTelemetryConfig,
			Prometheus: consuldp.PrometheusTelemetryConfig{
				RetentionTime: DefaultPromRetentionTime,
				ScrapePath:    DefaultPromScrapePath,
				MergePort:     DefaultPromMergePort,
			},
		},
		Envoy: &consuldp.EnvoyConfig{
			AdminBindAddress:              DefaultEnvoyAdminBindAddr,
			AdminBindPort:                 DefaultEnvoyAdminBindPort,
			ReadyBindPort:                 DefaultEnvoyReadyBindPort,
			EnvoyConcurrency:              DefaultEnvoyConcurrency,
			EnvoyDrainTimeSeconds:         DefaultEnvoyDrainTimeSeconds,
			EnvoyDrainStrategy:            DefaultEnvoyDrainStrategy,
			ShutdownDrainListenersEnabled: DefaultEnvoyShutdownDrainListenersEnabled,
			ShutdownGracePeriodSeconds:    DefaultEnvoyShutdownGracePeriodSeconds,
			GracefulShutdownPath:          DefaultGracefulShutdownPath,
			GracefulPort:                  DefaultGracefulPort,
			DumpEnvoyConfigOnExitEnabled:  DefaultDumpEnvoyConfigOnExitEnabled,
			ExtraArgs:                     []string{},
		},
		XDSServer: &consuldp.XDSServer{
			BindAddress: DefaultXDSBindAddr,
			BindPort:    DefaultXDSBindPort,
		},
		DNSServer: &consuldp.DNSServerConfig{
			BindAddr: DefaultDNSBindAddr,
			Port:     DefaultDNSBindPort,
		},
	}
}
