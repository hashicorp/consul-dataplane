// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/consul-dataplane/pkg/consuldp"
	"github.com/hashicorp/consul-dataplane/pkg/version"
)

var (
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

	adminBindAddr    string
	adminBindPort    int
	readyBindAddr    string
	readyBindPort    int
	envoyConcurrency int

	xdsBindAddr string
	xdsBindPort int

	consulDNSBindAddr string
	consulDNSPort     int

	shutdownDrainListeners bool
	shutdownGracePeriod    int
	gracefulShutdownPath   string
	gracefulPort           int
)

func init() {
	flag.BoolVar(&printVersion, "version", false, "Prints the current version of consul-dataplane.")

	StringVar(&addresses, "addresses", "", "DP_CONSUL_ADDRESSES", "Consul server gRPC addresses. Value can be:\n"+
		"1. A DNS name that resolves to server addresses or the DNS name of a load balancer in front of the Consul servers; OR\n"+
		"2. An executable command in the format, 'exec=<executable with optional args>'. The executable\n"+
		"	a) on success - should exit 0 and print to stdout whitespace delimited IP (v4/v6) addresses\n"+
		"	b) on failure - exit with a non-zero code and optionally print an error message of up to 1024 bytes to stderr.\n"+
		"	Refer to https://github.com/hashicorp/go-netaddrs#summary for more details and examples.\n")

	IntVar(&grpcPort, "grpc-port", 8502, "DP_CONSUL_GRPC_PORT", "The Consul server gRPC port to which consul-dataplane connects.")

	BoolVar(&serverWatchDisabled, "server-watch-disabled", false, "DP_SERVER_WATCH_DISABLED", "Setting this prevents consul-dataplane from consuming the server update stream. This is useful for situations where Consul servers are behind a load balancer.")

	StringVar(&logLevel, "log-level", "info", "DP_LOG_LEVEL", "Log level of the messages to print. "+
		"Available log levels are \"trace\", \"debug\", \"info\", \"warn\", and \"error\".")

	BoolVar(&logJSON, "log-json", false, "DP_LOG_JSON", "Enables log messages in JSON format.")

	StringVar(&nodeName, "service-node-name", "", "DP_SERVICE_NODE_NAME", "The name of the Consul node to which the proxy service instance is registered.")
	StringVar(&nodeID, "service-node-id", "", "DP_SERVICE_NODE_ID", "The ID of the Consul node to which the proxy service instance is registered.")
	StringVar(&serviceID, "proxy-service-id", "", "DP_PROXY_SERVICE_ID", "The proxy service instance's ID.")
	StringVar(&serviceIDPath, "proxy-service-id-path", "", "DP_PROXY_SERVICE_ID_PATH", "The path to a file containing the proxy service instance's ID.")
	StringVar(&namespace, "service-namespace", "", "DP_SERVICE_NAMESPACE", "The Consul Enterprise namespace in which the proxy service instance is registered.")
	StringVar(&partition, "service-partition", "", "DP_SERVICE_PARTITION", "The Consul Enterprise partition in which the proxy service instance is registered.")

	StringVar(&credentialType, "credential-type", "", "DP_CREDENTIAL_TYPE", "The type of credentials, either static or login, used to authenticate with Consul servers.")
	StringVar(&token, "static-token", "", "DP_CREDENTIAL_STATIC_TOKEN", "The ACL token used to authenticate requests to Consul servers when -credential-type is set to static.")
	StringVar(&loginAuthMethod, "login-auth-method", "", "DP_CREDENTIAL_LOGIN_AUTH_METHOD", "The auth method used to log in.")
	StringVar(&loginNamespace, "login-namespace", "", "DP_CREDENTIAL_LOGIN_NAMESPACE", "The Consul Enterprise namespace containing the auth method.")
	StringVar(&loginPartition, "login-partition", "", "DP_CREDENTIAL_LOGIN_PARTITION", "The Consul Enterprise partition containing the auth method.")
	StringVar(&loginDatacenter, "login-datacenter", "", "DP_CREDENTIAL_LOGIN_DATACENTER", "The datacenter containing the auth method.")
	StringVar(&loginBearerToken, "login-bearer-token", "", "DP_CREDENTIAL_LOGIN_BEARER_TOKEN", "The bearer token presented to the auth method.")
	StringVar(&loginBearerTokenPath, "login-bearer-token-path", "", "DP_CREDENTIAL_LOGIN_BEARER_TOKEN_PATH", "The path to a file containing the bearer token presented to the auth method.")
	MapVar((*FlagMapValue)(&loginMeta), "login-meta", "DP_CREDENTIAL_LOGIN_META", `A set of key/value pairs to attach to the ACL token. Each pair is formatted as "<key>=<value>". This flag may be passed multiple times.`)

	BoolVar(&useCentralTelemetryConfig, "telemetry-use-central-config", true, "DP_TELEMETRY_USE_CENTRAL_CONFIG", "Controls whether the proxy applies the central telemetry configuration.")

	DurationVar(&promRetentionTime, "telemetry-prom-retention-time", 60*time.Second, "DP_TELEMETRY_PROM_RETENTION_TIME", "The duration for prometheus metrics aggregation.")
	StringVar(&promCACertsPath, "telemetry-prom-ca-certs-path", "", "DP_TELEMETRY_PROM_CA_CERTS_PATH", "The path to a file or directory containing CA certificates used to verify the Prometheus server's certificate.")
	StringVar(&promKeyFile, "telemetry-prom-key-file", "", "DP_TELEMETRY_PROM_KEY_FILE", "The path to the client private key used to serve Prometheus metrics.")
	StringVar(&promCertFile, "telemetry-prom-cert-file", "", "DP_TELEMETRY_PROM_CERT_FILE", "The path to the client certificate used to serve Prometheus metrics.")
	StringVar(&promServiceMetricsURL, "telemetry-prom-service-metrics-url", "", "DP_TELEMETRY_PROM_SERVICE_METRICS_URL", "Prometheus metrics at this URL are scraped and included in Consul Dataplane's main Prometheus metrics.")
	StringVar(&promScrapePath, "telemetry-prom-scrape-path", "/metrics", "DP_TELEMETRY_PROM_SCRAPE_PATH", "The URL path where Envoy serves Prometheus metrics.")
	IntVar(&promMergePort, "telemetry-prom-merge-port", 20100, "DP_TELEMETRY_PROM_MERGE_PORT", "The port to serve merged Prometheus metrics.")

	StringVar(&adminBindAddr, "envoy-admin-bind-address", "127.0.0.1", "DP_ENVOY_ADMIN_BIND_ADDRESS", "The address on which the Envoy admin server is available.")
	IntVar(&adminBindPort, "envoy-admin-bind-port", 19000, "DP_ENVOY_ADMIN_BIND_PORT", "The port on which the Envoy admin server is available.")
	StringVar(&readyBindAddr, "envoy-ready-bind-address", "", "DP_ENVOY_READY_BIND_ADDRESS", "The address on which Envoy's readiness probe is available.")
	IntVar(&readyBindPort, "envoy-ready-bind-port", 0, "DP_ENVOY_READY_BIND_PORT", "The port on which Envoy's readiness probe is available.")
	IntVar(&envoyConcurrency, "envoy-concurrency", 2, "DP_ENVOY_CONCURRENCY", "The number of worker threads that Envoy uses.")

	StringVar(&xdsBindAddr, "xds-bind-addr", "127.0.0.1", "DP_XDS_BIND_ADDR", "The address on which the Envoy xDS server is available.")
	IntVar(&xdsBindPort, "xds-bind-port", 0, "DP_XDS_BIND_PORT", "The port on which the Envoy xDS server is available.")

	BoolVar(&tlsDisabled, "tls-disabled", false, "DP_TLS_DISABLED", "Communicate with Consul servers over a plaintext connection. Useful for testing, but not recommended for production.")
	StringVar(&tlsCACertsPath, "ca-certs", "", "DP_CA_CERTS", "The path to a file or directory containing CA certificates used to verify the server's certificate.")
	StringVar(&tlsCertFile, "tls-cert", "", "DP_TLS_CERT", "The path to a client certificate file. This is required if tls.grpc.verify_incoming is enabled on the server.")
	StringVar(&tlsKeyFile, "tls-key", "", "DP_TLS_KEY", "The path to a client private key file. This is required if tls.grpc.verify_incoming is enabled on the server.")
	StringVar(&tlsServerName, "tls-server-name", "", "DP_TLS_SERVER_NAME", "The hostname to expect in the server certificate's subject. This is required if -addresses is not a DNS name.")
	BoolVar(&tlsInsecureSkipVerify, "tls-insecure-skip-verify", false, "DP_TLS_INSECURE_SKIP_VERIFY", "Do not verify the server's certificate. Useful for testing, but not recommended for production.")

	StringVar(&consulDNSBindAddr, "consul-dns-bind-addr", "127.0.0.1", "DP_CONSUL_DNS_BIND_ADDR", "The address that will be bound to the consul dns proxy.")
	IntVar(&consulDNSPort, "consul-dns-bind-port", -1, "DP_CONSUL_DNS_BIND_PORT", "The port the consul dns proxy will listen on. By default -1 disables the dns proxy")

	// Default is false because it will generally be configured appropriately by Helm
	// configuration or pod annotation.
	BoolVar(&shutdownDrainListeners, "shutdown-drain-listeners", false, "DP_SHUTDOWN_DRAIN_LISTENERS", "Wait for proxy listeners to drain before terminating the proxy container.")
	// TODO: Should the grace period be implemented as a minimum or maximum? If all
	// connections have drained from the proxy before the end of the grace period,
	// should it terminate earlier?
	// Default is 0 because it will generally be configured appropriately by Helm
	// configuration or pod annotation.
	IntVar(&shutdownGracePeriod, "shutdown-grace-period", 0, "DP_SHUTDOWN_GRACE_PERIOD", "Amount of time to wait after receiving a SIGTERM signal before terminating the proxy.")
	StringVar(&gracefulShutdownPath, "graceful-shutdown-path", "/graceful_shutdown", "DP_GRACEFUL_SHUTDOWN_PATH", "An HTTP path to serve the graceful shutdown endpoint.")
	IntVar(&gracefulPort, "graceful-port", 20300, "DP_GRACEFUL_PORT", "A port to serve HTTP endpoints for graceful shutdown.")
}

// validateFlags performs semantic validation of the flag values
func validateFlags() {
	switch strings.ToUpper(logLevel) {
	case "TRACE", "DEBUG", "INFO", "WARN", "ERROR":
	default:
		log.Fatal("invalid log level. valid values - TRACE, DEBUG, INFO, WARN, ERROR")
	}
}

func main() {
	flag.Parse()

	if printVersion {
		fmt.Printf("Consul Dataplane v%s\n", version.GetHumanVersion())
		fmt.Printf("Revision %s\n", version.GitCommit)
		return
	}

	readServiceIDFromFile()
	validateFlags()

	consuldpCfg := &consuldp.Config{
		Consul: &consuldp.ConsulConfig{
			Addresses: addresses,
			GRPCPort:  grpcPort,
			Credentials: &consuldp.CredentialsConfig{
				Type: consuldp.CredentialsType(credentialType),
				Static: consuldp.StaticCredentialsConfig{
					Token: token,
				},
				Login: consuldp.LoginCredentialsConfig{
					AuthMethod:      loginAuthMethod,
					Namespace:       loginNamespace,
					Partition:       loginPartition,
					Datacenter:      loginDatacenter,
					BearerToken:     loginBearerToken,
					BearerTokenPath: loginBearerTokenPath,
					Meta:            loginMeta,
				},
			},
			ServerWatchDisabled: serverWatchDisabled,
			TLS: &consuldp.TLSConfig{
				Disabled:           tlsDisabled,
				CACertsPath:        tlsCACertsPath,
				ServerName:         tlsServerName,
				CertFile:           tlsCertFile,
				KeyFile:            tlsKeyFile,
				InsecureSkipVerify: tlsInsecureSkipVerify,
			},
		},
		Service: &consuldp.ServiceConfig{
			NodeName:  nodeName,
			NodeID:    nodeID,
			ServiceID: serviceID,
			Namespace: namespace,
			Partition: partition,
		},
		Logging: &consuldp.LoggingConfig{
			Name:     "consul-dataplane",
			LogLevel: strings.ToUpper(logLevel),
			LogJSON:  logJSON,
		},
		Telemetry: &consuldp.TelemetryConfig{
			UseCentralConfig: useCentralTelemetryConfig,
			Prometheus: consuldp.PrometheusTelemetryConfig{
				RetentionTime:     promRetentionTime,
				CACertsPath:       promCACertsPath,
				KeyFile:           promKeyFile,
				CertFile:          promCertFile,
				ServiceMetricsURL: promServiceMetricsURL,
				ScrapePath:        promScrapePath,
				MergePort:         promMergePort,
			},
		},
		Envoy: &consuldp.EnvoyConfig{
			AdminBindAddress: adminBindAddr,
			AdminBindPort:    adminBindPort,
			ReadyBindAddress: readyBindAddr,
			ReadyBindPort:    readyBindPort,
			EnvoyConcurrency: envoyConcurrency,
			ExtraArgs:        flag.Args(),
		},
		XDSServer: &consuldp.XDSServer{
			BindAddress: xdsBindAddr,
			BindPort:    xdsBindPort,
		},
		DNSServer: &consuldp.DNSServerConfig{
			BindAddr: consulDNSBindAddr,
			Port:     consulDNSPort,
		},
	}

	consuldpInstance, err := consuldp.NewConsulDP(consuldpCfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		cancel()
	}()

	err = consuldpInstance.Run(ctx)
	if err != nil {
		cancel()
		log.Fatal(err)
	}
}

// readServiceIDFromFile reads the service ID from the file specified by the
// -proxy-service-id-path flag.
//
// We do this here, rather than in the consuldp package's config handling,
// because this option only really makes sense as a CLI flag (and we handle
// all flag parsing here).
func readServiceIDFromFile() {
	if serviceID == "" && serviceIDPath != "" {
		id, err := os.ReadFile(serviceIDPath)
		if err != nil {
			log.Fatalf("failed to read given -proxy-service-id-path: %v", err)
		}
		serviceID = string(id)
	}
}
