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
	flags    *flag.FlagSet
)

func init() {
	flags = flag.NewFlagSet("", flag.ContinueOnError)
	flagOpts = &FlagOpts{}
	flags.BoolVar(&flagOpts.printVersion, "version", false, "Prints the current version of consul-dataplane.")

	StringVar(flags, &flagOpts.dataplaneConfig.Consul.Addresses, "addresses", "DP_CONSUL_ADDRESSES", "Consul server gRPC addresses. Value can be:\n"+
		"1. A DNS name that resolves to server addresses or the DNS name of a load balancer in front of the Consul servers; OR\n"+
		"2. An executable command in the format, 'exec=<executable with optional args>'. The executable\n"+
		"	a) on success - should exit 0 and print to stdout whitespace delimited IP (v4/v6) addresses\n"+
		"	b) on failure - exit with a non-zero code and optionally print an error message of up to 1024 bytes to stderr.\n"+
		"	Refer to https://github.com/hashicorp/go-netaddrs#summary for more details and examples.\n")

	IntVar(flags, &flagOpts.dataplaneConfig.Consul.GRPCPort, "grpc-port", "DP_CONSUL_GRPC_PORT", "The Consul server gRPC port to which consul-dataplane connects.")

	BoolVar(flags, &flagOpts.dataplaneConfig.Consul.ServerWatchDisabled, "server-watch-disabled", "DP_SERVER_WATCH_DISABLED", "Setting this prevents consul-dataplane from consuming the server update stream. This is useful for situations where Consul servers are behind a load balancer.")

	StringVar(flags, &flagOpts.dataplaneConfig.Logging.LogLevel, "log-level", "DP_LOG_LEVEL", "Log level of the messages to print. "+
		"Available log levels are \"trace\", \"debug\", \"info\", \"warn\", and \"error\".")

	BoolVar(flags, &flagOpts.dataplaneConfig.Logging.LogJSON, "log-json", "DP_LOG_JSON", "Enables log messages in JSON format.")

	StringVar(flags, &flagOpts.dataplaneConfig.Service.NodeName, "service-node-name", "DP_SERVICE_NODE_NAME",
		"[Deprecated; use -proxy-node-name instead] The name of the Consul node to which the proxy service instance is registered.")
	StringVar(flags, &flagOpts.dataplaneConfig.Service.NodeID, "service-node-id", "DP_SERVICE_NODE_ID",
		"[Deprecated; use -proxy-node-id instead] The ID of the Consul node to which the proxy service instance is registered.")
	StringVar(flags, &flagOpts.dataplaneConfig.Service.ServiceID, "proxy-service-id", "DP_PROXY_SERVICE_ID",
		"[Deprecated; use -proxy-id instead] The proxy service instance's ID.")
	StringVar(flags, &flagOpts.dataplaneConfig.Service.ServiceIDPath, "proxy-service-id-path", "DP_PROXY_SERVICE_ID_PATH",
		"[Deprecated; use -proxy-id-path instead] The path to a file containing the proxy service instance's ID.")
	StringVar(flags, &flagOpts.dataplaneConfig.Service.Namespace, "service-namespace", "DP_SERVICE_NAMESPACE",
		"[Deprecated; use -proxy-namespace instead] The Consul Enterprise namespace in which the proxy service instance is registered.")
	StringVar(flags, &flagOpts.dataplaneConfig.Service.Partition, "service-partition", "DP_SERVICE_PARTITION",
		"[Deprecated; use -proxy-partition instead] The Consul Enterprise partition in which the proxy service instance is registered.")

	StringVar(flags, &flagOpts.dataplaneConfig.Proxy.NodeName, "proxy-node-name", "DP_PROXY_NODE_NAME",
		"The name of the Consul node to which the proxy service instance is registered."+
			"In Consul's V2 Catalog API, this value is ignored.")
	StringVar(flags, &flagOpts.dataplaneConfig.Proxy.NodeID, "proxy-node-id", "DP_PROXY_NODE_ID",
		"The ID of the Consul node to which the proxy service instance is registered."+
			"In Consul's V2 Catalog API, this value is ignored.")
	StringVar(flags, &flagOpts.dataplaneConfig.Proxy.ID, "proxy-id", "DP_PROXY_ID",
		"In Consul's V1 Catalog API, the proxy service instance's ID."+
			"In Consul's V2 Catalog API, the workload ID associated with the proxy.")
	StringVar(flags, &flagOpts.dataplaneConfig.Proxy.IDPath, "proxy-id-path", "DP_PROXY_ID_PATH",
		"In Consul's V1 Catalog API, the path to a file containing the proxy service instance's ID."+
			"In Consul's V2 Catalog API, the path to a file containing the workload ID associated with the proxy.")
	StringVar(flags, &flagOpts.dataplaneConfig.Proxy.Namespace, "proxy-namespace", "DP_PROXY_NAMESPACE",
		"The Consul Enterprise namespace in which the proxy service instance (V1 API) or workload (V2 API) is registered.")
	StringVar(flags, &flagOpts.dataplaneConfig.Proxy.Partition, "proxy-partition", "DP_PROXY_PARTITION",
		"The Consul Enterprise partition in which the proxy service instance (V1 API) or workload (V2 API) is registered.")

	StringVar(flags, &flagOpts.dataplaneConfig.Consul.Credentials.Type, "credential-type", "DP_CREDENTIAL_TYPE", "The type of credentials, either static or login, used to authenticate with Consul servers.")
	StringVar(flags, &flagOpts.dataplaneConfig.Consul.Credentials.Static.Token, "static-token", "DP_CREDENTIAL_STATIC_TOKEN", "The ACL token used to authenticate requests to Consul servers when -credential-type is set to static.")
	StringVar(flags, &flagOpts.dataplaneConfig.Consul.Credentials.Login.AuthMethod, "login-auth-method", "DP_CREDENTIAL_LOGIN_AUTH_METHOD", "The auth method used to log in.")
	StringVar(flags, &flagOpts.dataplaneConfig.Consul.Credentials.Login.Namespace, "login-namespace", "DP_CREDENTIAL_LOGIN_NAMESPACE", "The Consul Enterprise namespace containing the auth method.")
	StringVar(flags, &flagOpts.dataplaneConfig.Consul.Credentials.Login.Partition, "login-partition", "DP_CREDENTIAL_LOGIN_PARTITION", "The Consul Enterprise partition containing the auth method.")
	StringVar(flags, &flagOpts.dataplaneConfig.Consul.Credentials.Login.Datacenter, "login-datacenter", "DP_CREDENTIAL_LOGIN_DATACENTER", "The datacenter containing the auth method.")
	StringVar(flags, &flagOpts.dataplaneConfig.Consul.Credentials.Login.BearerToken, "login-bearer-token", "DP_CREDENTIAL_LOGIN_BEARER_TOKEN", "The bearer token presented to the auth method.")
	StringVar(flags, &flagOpts.dataplaneConfig.Consul.Credentials.Login.BearerTokenPath, "login-bearer-token-path", "DP_CREDENTIAL_LOGIN_BEARER_TOKEN_PATH", "The path to a file containing the bearer token presented to the auth method.")
	MapVar(flags, (*FlagMapValue)(&flagOpts.dataplaneConfig.Consul.Credentials.Login.Meta), "login-meta", "DP_CREDENTIAL_LOGIN_META", `A set of key/value pairs to attach to the ACL token. Each pair is formatted as "<key>=<value>". This flag may be passed multiple times.`)

	BoolVar(flags, &flagOpts.dataplaneConfig.Telemetry.UseCentralConfig, "telemetry-use-central-config", "DP_TELEMETRY_USE_CENTRAL_CONFIG", "Controls whether the proxy applies the central telemetry configuration.")

	DurationVar(flags, &flagOpts.dataplaneConfig.Telemetry.Prometheus.RetentionTime, "telemetry-prom-retention-time", "DP_TELEMETRY_PROM_RETENTION_TIME", "The duration for prometheus metrics aggregation.")
	StringVar(flags, &flagOpts.dataplaneConfig.Telemetry.Prometheus.CACertsPath, "telemetry-prom-ca-certs-path", "DP_TELEMETRY_PROM_CA_CERTS_PATH", "The path to a file or directory containing CA certificates used to verify the Prometheus server's certificate.")
	StringVar(flags, &flagOpts.dataplaneConfig.Telemetry.Prometheus.KeyFile, "telemetry-prom-key-file", "DP_TELEMETRY_PROM_KEY_FILE", "The path to the client private key used to serve Prometheus metrics.")
	StringVar(flags, &flagOpts.dataplaneConfig.Telemetry.Prometheus.CertFile, "telemetry-prom-cert-file", "DP_TELEMETRY_PROM_CERT_FILE", "The path to the client certificate used to serve Prometheus metrics.")
	StringVar(flags, &flagOpts.dataplaneConfig.Telemetry.Prometheus.ServiceMetricsURL, "telemetry-prom-service-metrics-url", "DP_TELEMETRY_PROM_SERVICE_METRICS_URL", "Prometheus metrics at this URL are scraped and included in Consul Dataplane's main Prometheus metrics.")
	StringVar(flags, &flagOpts.dataplaneConfig.Telemetry.Prometheus.ScrapePath, "telemetry-prom-scrape-path", "DP_TELEMETRY_PROM_SCRAPE_PATH", "The URL path where Envoy serves Prometheus metrics.")
	IntVar(flags, &flagOpts.dataplaneConfig.Telemetry.Prometheus.MergePort, "telemetry-prom-merge-port", "DP_TELEMETRY_PROM_MERGE_PORT", "The port to serve merged Prometheus metrics.")

	StringVar(flags, &flagOpts.dataplaneConfig.Envoy.AdminBindAddr, "envoy-admin-bind-address", "DP_ENVOY_ADMIN_BIND_ADDRESS", "The address on which the Envoy admin server is available.")
	IntVar(flags, &flagOpts.dataplaneConfig.Envoy.AdminBindPort, "envoy-admin-bind-port", "DP_ENVOY_ADMIN_BIND_PORT", "The port on which the Envoy admin server is available.")
	StringVar(flags, &flagOpts.dataplaneConfig.Envoy.ReadyBindAddr, "envoy-ready-bind-address", "DP_ENVOY_READY_BIND_ADDRESS", "The address on which Envoy's readiness probe is available.")
	IntVar(flags, &flagOpts.dataplaneConfig.Envoy.ReadyBindPort, "envoy-ready-bind-port", "DP_ENVOY_READY_BIND_PORT", "The port on which Envoy's readiness probe is available.")
	IntVar(flags, &flagOpts.dataplaneConfig.Envoy.Concurrency, "envoy-concurrency", "DP_ENVOY_CONCURRENCY", "The number of worker threads that Envoy uses.")
	IntVar(flags, &flagOpts.dataplaneConfig.Envoy.DrainTimeSeconds, "envoy-drain-time-seconds", "DP_ENVOY_DRAIN_TIME", "The time in seconds for which Envoy will drain connections.")
	StringVar(flags, &flagOpts.dataplaneConfig.Envoy.DrainStrategy, "envoy-drain-strategy", "DP_ENVOY_DRAIN_STRATEGY", "The behaviour of Envoy during the drain sequence. Determines whether all open connections should be encouraged to drain immediately or to increase the percentage gradually as the drain time elapses.")

	StringVar(flags, &flagOpts.dataplaneConfig.XDSServer.BindAddr, "xds-bind-addr", "DP_XDS_BIND_ADDR", "The address on which the Envoy xDS server is available.")
	IntVar(flags, &flagOpts.dataplaneConfig.XDSServer.BindPort, "xds-bind-port", "DP_XDS_BIND_PORT", "The port on which the Envoy xDS server is available.")

	BoolVar(flags, &flagOpts.dataplaneConfig.Consul.TLS.Disabled, "tls-disabled", "DP_TLS_DISABLED", "Communicate with Consul servers over a plaintext connection. Useful for testing, but not recommended for production.")
	StringVar(flags, &flagOpts.dataplaneConfig.Consul.TLS.CACertsPath, "ca-certs", "DP_CA_CERTS", "The path to a file or directory containing CA certificates used to verify the server's certificate.")
	StringVar(flags, &flagOpts.dataplaneConfig.Consul.TLS.CertFile, "tls-cert", "DP_TLS_CERT", "The path to a client certificate file. This is required if tls.grpc.verify_incoming is enabled on the server.")
	StringVar(flags, &flagOpts.dataplaneConfig.Consul.TLS.KeyFile, "tls-key", "DP_TLS_KEY", "The path to a client private key file. This is required if tls.grpc.verify_incoming is enabled on the server.")
	StringVar(flags, &flagOpts.dataplaneConfig.Consul.TLS.ServerName, "tls-server-name", "DP_TLS_SERVER_NAME", "The hostname to expect in the server certificate's subject. This is required if -addresses is not a DNS name.")
	BoolVar(flags, &flagOpts.dataplaneConfig.Consul.TLS.InsecureSkipVerify, "tls-insecure-skip-verify", "DP_TLS_INSECURE_SKIP_VERIFY", "Do not verify the server's certificate. Useful for testing, but not recommended for production.")

	StringVar(flags, &flagOpts.dataplaneConfig.DNSServer.BindAddr, "consul-dns-bind-addr", "DP_CONSUL_DNS_BIND_ADDR", "The address that will be bound to the consul dns proxy.")
	IntVar(flags, &flagOpts.dataplaneConfig.DNSServer.BindPort, "consul-dns-bind-port", "DP_CONSUL_DNS_BIND_PORT", "The port the consul dns proxy will listen on. By default -1 disables the dns proxy")

	// Default is false because it will generally be configured appropriately by Helm
	// configuration or pod annotation.
	BoolVar(flags, &flagOpts.dataplaneConfig.Envoy.ShutdownDrainListenersEnabled, "shutdown-drain-listeners", "DP_SHUTDOWN_DRAIN_LISTENERS", "Wait for proxy listeners to drain before terminating the proxy container.")
	// Default is 0 because it will generally be configured appropriately by Helm
	// configuration or pod annotation.
	IntVar(flags, &flagOpts.dataplaneConfig.Envoy.ShutdownGracePeriodSeconds, "shutdown-grace-period-seconds", "DP_SHUTDOWN_GRACE_PERIOD_SECONDS", "Amount of time to wait after receiving a SIGTERM signal before terminating the proxy.")
	StringVar(flags, &flagOpts.dataplaneConfig.Envoy.GracefulShutdownPath, "graceful-shutdown-path", "DP_GRACEFUL_SHUTDOWN_PATH", "An HTTP path to serve the graceful shutdown endpoint.")
	IntVar(flags, &flagOpts.dataplaneConfig.Envoy.GracefulPort, "graceful-port", "DP_GRACEFUL_PORT", "A port to serve HTTP endpoints for graceful shutdown.")
	IntVar(flags, &flagOpts.dataplaneConfig.Envoy.StartupGracePeriodSeconds, "startup-grace-period-seconds", "DP_STARTUP_GRACE_PERIOD_SECONDS", "Amount of time to wait for consul-dataplane startup.")
	StringVar(flags, &flagOpts.dataplaneConfig.Envoy.GracefulStartupPath, "graceful-startup-path", "DP_GRACEFUL_STARTUP_PATH", "An HTTP path to serve the graceful startup endpoint.")
	// Default is false, may be useful for debugging unexpected termination.
	BoolVar(flags, &flagOpts.dataplaneConfig.Envoy.DumpEnvoyConfigOnExitEnabled, "dump-envoy-config-on-exit", "DP_DUMP_ENVOY_CONFIG_ON_EXIT", "Call the Envoy /config_dump endpoint during consul-dataplane controlled shutdown.")

	flags.StringVar(&flagOpts.configFile, "config-file", "", "The json config file for configuring consul data plane")
}

// validateFlags performs semantic validation of the flag values
func validateFlags() {
	if flagOpts.dataplaneConfig.Logging.LogLevel != nil {
		switch strings.ToUpper(*flagOpts.dataplaneConfig.Logging.LogLevel) {
		case "TRACE", "DEBUG", "INFO", "WARN", "ERROR":
		default:
			log.Fatal("invalid log level. valid values - TRACE, DEBUG, INFO, WARN, ERROR")
		}
	}

	if flagOpts.configFile != "" && !strings.HasSuffix(flagOpts.configFile, ".json") {
		log.Fatal("invalid config file format. Should be a json file")
	}
}

func run() error {
	// Shift arguments by one if subcommand is the first argument.
	subcommand := os.Args[1]
	var arguments []string
	switch subcommand {
	case "graceful-startup":
		arguments = os.Args[2:]
	default:
		arguments = os.Args[1:]
	}

	err := flags.Parse(arguments)
	if err != nil {
		return err
	}

	if flagOpts.printVersion {
		fmt.Printf("Consul Dataplane v%s\n", version.GetHumanVersion())
		fmt.Printf("Revision %s\n", version.GitCommit)
		return nil
	}

	readServiceIDFromFile()
	readProxyIDFromFile()
	validateFlags()

	consuldpCfg, err := flagOpts.buildDataplaneConfig(flags.Args())
	if err != nil {
		return err
	}

	if subcommand == "graceful-startup" {
		fmt.Println("graceful port is :", consuldpCfg.Envoy.GracefulPort)
		log.Default().Printf(fmt.Sprintf("graceful port is: %d", consuldpCfg.Envoy.GracefulPort))
		return RunGracefulStartup(consuldpCfg.Envoy.GracefulStartupPath, consuldpCfg.Envoy.GracefulPort)
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
		fmt.Println("Waiting for SIGINT or SIGTERM")
		// Block waiting for SIGINT or SIGTERM
		v := <-sigCh

		fmt.Println("Received signal", v)
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
	if flagOpts.dataplaneConfig.Service.ServiceID == nil &&
		flagOpts.dataplaneConfig.Service.ServiceIDPath != nil &&
		*flagOpts.dataplaneConfig.Service.ServiceIDPath != "" {
		id, err := os.ReadFile(*flagOpts.dataplaneConfig.Service.ServiceIDPath)
		if err != nil {
			log.Fatalf("failed to read given -proxy-service-id-path: %v", err)
		}
		s := string(id)
		flagOpts.dataplaneConfig.Service.ServiceID = &s
	}
}

// readProxyIDFromFile reads the proxy ID from the file specified by the
// -proxy-id-path flag.
//
// We do this here, rather than in the consuldp package's config handling,
// because this option only really makes sense as a CLI flag (and we handle
// all flag parsing here).
func readProxyIDFromFile() {
	if flagOpts.dataplaneConfig.Proxy.ID == nil &&
		flagOpts.dataplaneConfig.Proxy.IDPath != nil &&
		*flagOpts.dataplaneConfig.Proxy.IDPath != "" {
		id, err := os.ReadFile(*flagOpts.dataplaneConfig.Proxy.IDPath)
		if err != nil {
			log.Fatalf("failed to read given -proxy-id-path: %v", err)
		}
		s := string(id)
		flagOpts.dataplaneConfig.Proxy.ID = &s
	}
}
