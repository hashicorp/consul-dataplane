package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/hashicorp/consul-dataplane/pkg/consuldp"
)

var (
	addresses string
	grpcPort  int

	logLevel string
	logJSON  bool

	nodeName  string
	nodeID    string
	serviceID string
	namespace string
	partition string

	token string

	useCentralTelemetryConfig bool

	adminBindAddr string
	adminBindPort int
	readyBindAddr string
	readyBindPort int

	xdsBindAddr string
	xdsBindPort int
)

func init() {
	flag.StringVar(&addresses, "addresses", "", "Consul server addresses. Value can be:\n"+
		"1. DNS name (that resolves to servers or DNS name of a load-balancer front of Consul servers); OR\n"+
		"2.'exec=<executable with optional args>'. The executable\n"+
		"	a) on success - should exit 0 and print to stdout whitespace delimited IP (v4/v6) addresses\n"+
		"	b) on failure - exit with a non-zero code and optionally print an error message of upto 1024 bytes to stderr.\n"+
		"	Refer to https://github.com/hashicorp/go-netaddrs#summary for more details and examples.")

	flag.IntVar(&grpcPort, "grpc-port", 8502, "gRPC port on Consul servers.")

	flag.StringVar(&logLevel, "log-level", "info", "Log level of the messages to print. "+
		"Available log levels are \"trace\", \"debug\", \"info\", \"warn\", and \"error\".")

	flag.BoolVar(&logJSON, "log-json", false, "Controls consul-dataplane logging in JSON format. By default this is false.")

	flag.StringVar(&nodeName, "service-node-name", "", "The name of the node to which the proxy service instance is registered.")
	flag.StringVar(&nodeID, "service-node-id", "", "The ID of the node to which the proxy service instance is registered.")
	flag.StringVar(&serviceID, "proxy-service-id", "", "The proxy service instance's ID.")
	flag.StringVar(&namespace, "service-namespace", "", "The Consul Enterprise namespace in which the proxy service instance is registered.")
	flag.StringVar(&partition, "service-partition", "", "The Consul Enterprise partition in which the proxy service instance is registered.")

	flag.StringVar(&token, "static-token", "", "The ACL token used to authenticate requests to Consul servers (when -login-method is set to static).")

	flag.BoolVar(&useCentralTelemetryConfig, "telemetry-use-central-config", true, "Controls whether the proxy will apply the central telemetry configuration.")

	flag.StringVar(&adminBindAddr, "envoy-admin-bind-address", "127.0.0.1", "The address on which the Envoy admin server will be available.")
	flag.IntVar(&adminBindPort, "envoy-admin-bind-port", 19000, "The port on which the Envoy admin server will be available.")
	flag.StringVar(&readyBindAddr, "envoy-ready-bind-address", "", "The address on which Envoy's readiness probe will be available.")
	flag.IntVar(&readyBindPort, "envoy-ready-bind-port", 0, "The port on which Envoy's readiness probe will be available.")

	flag.StringVar(&xdsBindAddr, "xds-bind-addr", "127.0.0.1", "The address on which the Envoy xDS server will be available.")
	flag.IntVar(&xdsBindPort, "xds-bind-port", 0, "The port on which the Envoy xDS server will be available.")
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

	validateFlags()

	consuldpCfg := &consuldp.Config{
		Consul: &consuldp.ConsulConfig{
			Addresses: addresses,
			GRPCPort:  grpcPort,
			Credentials: &consuldp.CredentialsConfig{
				Static: &consuldp.StaticCredentialsConfig{
					Token: token,
				},
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
		},
		Envoy: &consuldp.EnvoyConfig{
			AdminBindAddress: adminBindAddr,
			AdminBindPort:    adminBindPort,
			ReadyBindAddress: readyBindAddr,
			ReadyBindPort:    readyBindPort,
		},
		XDSServer: &consuldp.XDSServer{
			BindAddress: xdsBindAddr,
			BindPort:    xdsBindPort,
		},
	}
	consuldpInstance, err := consuldp.NewConsulDP(consuldpCfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		<-sigCh
		cancel()
	}()

	err = consuldpInstance.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
}
