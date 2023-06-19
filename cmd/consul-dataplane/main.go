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

	"github.com/hashicorp/consul-dataplane/pkg/consuldp"
	"github.com/hashicorp/consul-dataplane/pkg/version"
)

var (
	flagOpts *FlagOpts
)

func init() {
	flagOpts = &FlagOpts{}
	flag.BoolVar(&flagOpts.printVersion, "version", false, "Prints the current version of consul-dataplane.")

	StringVar(&flagOpts.addresses, "addresses", "", "DP_CONSUL_ADDRESSES", "Consul server gRPC addresses. Value can be:\n"+
		"1. A DNS name that resolves to server addresses or the DNS name of a load balancer in front of the Consul servers; OR\n"+
		"2. An executable command in the format, 'exec=<executable with optional args>'. The executable\n"+
		"	a) on success - should exit 0 and print to stdout whitespace delimited IP (v4/v6) addresses\n"+
		"	b) on failure - exit with a non-zero code and optionally print an error message of up to 1024 bytes to stderr.\n"+
		"	Refer to https://github.com/hashicorp/go-netaddrs#summary for more details and examples.\n")

	IntVar(&flagOpts.grpcPort, "grpc-port", DefaultGRPCPort, "DP_CONSUL_GRPC_PORT", "The Consul server gRPC port to which consul-dataplane connects.")

	BoolVar(&flagOpts.serverWatchDisabled, "server-watch-disabled", DefaultServerWatchDisabled, "DP_SERVER_WATCH_DISABLED", "Setting this prevents consul-dataplane from consuming the server update stream. This is useful for situations where Consul servers are behind a load balancer.")

	StringVar(&flagOpts.logLevel, "log-level", DefaultLogLevel, "DP_LOG_LEVEL", "Log level of the messages to print. "+
		"Available log levels are \"trace\", \"debug\", \"info\", \"warn\", and \"error\".")

	BoolVar(&flagOpts.logJSON, "log-json", DefaultLogJSON, "DP_LOG_JSON", "Enables log messages in JSON format.")

	StringVar(&flagOpts.nodeName, "service-node-name", "", "DP_SERVICE_NODE_NAME", "The name of the Consul node to which the proxy service instance is registered.")
	StringVar(&flagOpts.nodeID, "service-node-id", "", "DP_SERVICE_NODE_ID", "The ID of the Consul node to which the proxy service instance is registered.")
	StringVar(&flagOpts.serviceID, "proxy-service-id", "", "DP_PROXY_SERVICE_ID", "The proxy service instance's ID.")
	StringVar(&flagOpts.serviceIDPath, "proxy-service-id-path", "", "DP_PROXY_SERVICE_ID_PATH", "The path to a file containing the proxy service instance's ID.")
	StringVar(&flagOpts.namespace, "service-namespace", "", "DP_SERVICE_NAMESPACE", "The Consul Enterprise namespace in which the proxy service instance is registered.")
	StringVar(&flagOpts.partition, "service-partition", "", "DP_SERVICE_PARTITION", "The Consul Enterprise partition in which the proxy service instance is registered.")

	StringVar(&flagOpts.credentialType, "credential-type", "", "DP_CREDENTIAL_TYPE", "The type of credentials, either static or login, used to authenticate with Consul servers.")
	StringVar(&flagOpts.token, "static-token", "", "DP_CREDENTIAL_STATIC_TOKEN", "The ACL token used to authenticate requests to Consul servers when -credential-type is set to static.")
	StringVar(&flagOpts.loginAuthMethod, "login-auth-method", "", "DP_CREDENTIAL_LOGIN_AUTH_METHOD", "The auth method used to log in.")
	StringVar(&flagOpts.loginNamespace, "login-namespace", "", "DP_CREDENTIAL_LOGIN_NAMESPACE", "The Consul Enterprise namespace containing the auth method.")
	StringVar(&flagOpts.loginPartition, "login-partition", "", "DP_CREDENTIAL_LOGIN_PARTITION", "The Consul Enterprise partition containing the auth method.")
	StringVar(&flagOpts.loginDatacenter, "login-datacenter", "", "DP_CREDENTIAL_LOGIN_DATACENTER", "The datacenter containing the auth method.")
	StringVar(&flagOpts.loginBearerToken, "login-bearer-token", "", "DP_CREDENTIAL_LOGIN_BEARER_TOKEN", "The bearer token presented to the auth method.")
	StringVar(&flagOpts.loginBearerTokenPath, "login-bearer-token-path", "", "DP_CREDENTIAL_LOGIN_BEARER_TOKEN_PATH", "The path to a file containing the bearer token presented to the auth method.")
	MapVar((*FlagMapValue)(&flagOpts.loginMeta), "login-meta", "DP_CREDENTIAL_LOGIN_META", `A set of key/value pairs to attach to the ACL token. Each pair is formatted as "<key>=<value>". This flag may be passed multiple times.`)

	BoolVar(&flagOpts.useCentralTelemetryConfig, "telemetry-use-central-config", DefaultUseCentralTelemetryConfig, "DP_TELEMETRY_USE_CENTRAL_CONFIG", "Controls whether the proxy applies the central telemetry configuration.")

	DurationVar(&flagOpts.promRetentionTime, "telemetry-prom-retention-time", DefaultPromRetentionTime, "DP_TELEMETRY_PROM_RETENTION_TIME", "The duration for prometheus metrics aggregation.")
	StringVar(&flagOpts.promCACertsPath, "telemetry-prom-ca-certs-path", "", "DP_TELEMETRY_PROM_CA_CERTS_PATH", "The path to a file or directory containing CA certificates used to verify the Prometheus server's certificate.")
	StringVar(&flagOpts.promKeyFile, "telemetry-prom-key-file", "", "DP_TELEMETRY_PROM_KEY_FILE", "The path to the client private key used to serve Prometheus metrics.")
	StringVar(&flagOpts.promCertFile, "telemetry-prom-cert-file", "", "DP_TELEMETRY_PROM_CERT_FILE", "The path to the client certificate used to serve Prometheus metrics.")
	StringVar(&flagOpts.promServiceMetricsURL, "telemetry-prom-service-metrics-url", "", "DP_TELEMETRY_PROM_SERVICE_METRICS_URL", "Prometheus metrics at this URL are scraped and included in Consul Dataplane's main Prometheus metrics.")
	StringVar(&flagOpts.promScrapePath, "telemetry-prom-scrape-path", DefaultPromScrapePath, "DP_TELEMETRY_PROM_SCRAPE_PATH", "The URL path where Envoy serves Prometheus metrics.")
	IntVar(&flagOpts.promMergePort, "telemetry-prom-merge-port", DefaultPromMergePort, "DP_TELEMETRY_PROM_MERGE_PORT", "The port to serve merged Prometheus metrics.")

	StringVar(&flagOpts.adminBindAddr, "envoy-admin-bind-address", DefaultEnvoyAdminBindAddr, "DP_ENVOY_ADMIN_BIND_ADDRESS", "The address on which the Envoy admin server is available.")
	IntVar(&flagOpts.adminBindPort, "envoy-admin-bind-port", DefaultEnvoyAdminBindPort, "DP_ENVOY_ADMIN_BIND_PORT", "The port on which the Envoy admin server is available.")
	StringVar(&flagOpts.readyBindAddr, "envoy-ready-bind-address", "", "DP_ENVOY_READY_BIND_ADDRESS", "The address on which Envoy's readiness probe is available.")
	IntVar(&flagOpts.readyBindPort, "envoy-ready-bind-port", DefaultEnvoyReadyBindPort, "DP_ENVOY_READY_BIND_PORT", "The port on which Envoy's readiness probe is available.")
	IntVar(&flagOpts.envoyConcurrency, "envoy-concurrency", DefaultEnvoyConcurrency, "DP_ENVOY_CONCURRENCY", "The number of worker threads that Envoy uses.")
	IntVar(&flagOpts.envoyDrainTimeSeconds, "envoy-drain-time-seconds", DefaultEnvoyDrainTimeSeconds, "DP_ENVOY_DRAIN_TIME", "The time in seconds for which Envoy will drain connections.")
	StringVar(&flagOpts.envoyDrainStrategy, "envoy-drain-strategy", DefaultEnvoyDrainStrategy, "DP_ENVOY_DRAIN_STRATEGY", "The behaviour of Envoy during the drain sequence. Determines whether all open connections should be encouraged to drain immediately or to increase the percentage gradually as the drain time elapses.")

	StringVar(&flagOpts.xdsBindAddr, "xds-bind-addr", DefaultXDSBindAddr, "DP_XDS_BIND_ADDR", "The address on which the Envoy xDS server is available.")
	IntVar(&flagOpts.xdsBindPort, "xds-bind-port", DefaultXDSBindPort, "DP_XDS_BIND_PORT", "The port on which the Envoy xDS server is available.")

	BoolVar(&flagOpts.tlsDisabled, "tls-disabled", DefaultTLSDisabled, "DP_TLS_DISABLED", "Communicate with Consul servers over a plaintext connection. Useful for testing, but not recommended for production.")
	StringVar(&flagOpts.tlsCACertsPath, "ca-certs", "", "DP_CA_CERTS", "The path to a file or directory containing CA certificates used to verify the server's certificate.")
	StringVar(&flagOpts.tlsCertFile, "tls-cert", "", "DP_TLS_CERT", "The path to a client certificate file. This is required if tls.grpc.verify_incoming is enabled on the server.")
	StringVar(&flagOpts.tlsKeyFile, "tls-key", "", "DP_TLS_KEY", "The path to a client private key file. This is required if tls.grpc.verify_incoming is enabled on the server.")
	StringVar(&flagOpts.tlsServerName, "tls-server-name", "", "DP_TLS_SERVER_NAME", "The hostname to expect in the server certificate's subject. This is required if -addresses is not a DNS name.")
	BoolVar(&flagOpts.tlsInsecureSkipVerify, "tls-insecure-skip-verify", DefaultTLSInsecureSkipVerify, "DP_TLS_INSECURE_SKIP_VERIFY", "Do not verify the server's certificate. Useful for testing, but not recommended for production.")

	StringVar(&flagOpts.consulDNSBindAddr, "consul-dns-bind-addr", DefaultDNSBindAddr, "DP_CONSUL_DNS_BIND_ADDR", "The address that will be bound to the consul dns proxy.")
	IntVar(&flagOpts.consulDNSPort, "consul-dns-bind-port", DefaultDNSBindPort, "DP_CONSUL_DNS_BIND_PORT", "The port the consul dns proxy will listen on. By default -1 disables the dns proxy")

	// Default is false because it will generally be configured appropriately by Helm
	// configuration or pod annotation.
	BoolVar(&flagOpts.shutdownDrainListenersEnabled, "shutdown-drain-listeners", DefaultEnvoyShutdownDrainListenersEnabled, "DP_SHUTDOWN_DRAIN_LISTENERS", "Wait for proxy listeners to drain before terminating the proxy container.")
	// Default is 0 because it will generally be configured appropriately by Helm
	// configuration or pod annotation.
	IntVar(&flagOpts.shutdownGracePeriodSeconds, "shutdown-grace-period-seconds", DefaultEnvoyShutdownGracePeriodSeconds, "DP_SHUTDOWN_GRACE_PERIOD_SECONDS", "Amount of time to wait after receiving a SIGTERM signal before terminating the proxy.")
	StringVar(&flagOpts.gracefulShutdownPath, "graceful-shutdown-path", DefaultGracefulShutdownPath, "DP_GRACEFUL_SHUTDOWN_PATH", "An HTTP path to serve the graceful shutdown endpoint.")
	IntVar(&flagOpts.gracefulPort, "graceful-port", DefaultGracefulPort, "DP_GRACEFUL_PORT", "A port to serve HTTP endpoints for graceful shutdown.")

	// Default is false, may be useful for debugging unexpected termination.
	BoolVar(&flagOpts.dumpEnvoyConfigOnExitEnabled, "dump-envoy-config-on-exit", DefaultDumpEnvoyConfigOnExitEnabled, "DP_DUMP_ENVOY_CONFIG_ON_EXIT", "Call the Envoy /config_dump endpoint during consul-dataplane controlled shutdown.")

	StringVar(&flagOpts.configFile, "config-file", "", "DP_CONFIG_FILE", "The json config file for configuring consul data plane")
}

// validateFlags performs semantic validation of the flag values
func validateFlags() {
	switch strings.ToUpper(flagOpts.logLevel) {
	case "TRACE", "DEBUG", "INFO", "WARN", "ERROR":
	default:
		log.Fatal("invalid log level. valid values - TRACE, DEBUG, INFO, WARN, ERROR")
	}

	if flagOpts.configFile != "" && !strings.HasSuffix(flagOpts.configFile, ".json") {
		log.Fatal("invalid config file format. Should be a json file")
	}
}

func run() error {
	flag.Parse()

	if flagOpts.printVersion {
		fmt.Printf("Consul Dataplane v%s\n", version.GetHumanVersion())
		fmt.Printf("Revision %s\n", version.GitCommit)
		return nil
	}

	readServiceIDFromFile()
	validateFlags()

	consuldpCfg, err := flagOpts.buildDataplaneConfig()
	if err != nil {
		return err
	}

	consuldpInstance, err := consuldp.NewConsulDP(consuldpCfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		// Block waiting for SIGTERM
		<-sigCh

		consuldpInstance.GracefulShutdown(cancel)
	}()

	return consuldpInstance.Run(ctx)
}

func main() {
	err := run()
	if err != nil {
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
	if flagOpts.serviceID == "" && flagOpts.serviceIDPath != "" {
		id, err := os.ReadFile(flagOpts.serviceIDPath)
		if err != nil {
			log.Fatalf("failed to read given -proxy-service-id-path: %v", err)
		}
		flagOpts.serviceID = string(id)
	}
}
