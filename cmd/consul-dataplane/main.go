package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

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

	nodeName  string
	nodeID    string
	serviceID string
	namespace string
	partition string

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

	adminBindAddr    string
	adminBindPort    int
	readyBindAddr    string
	readyBindPort    int
	envoyConcurrency int

	xdsBindAddr string
	xdsBindPort int

	consulDNSBindAddr string
	consulDNSPort     int
)

func init() {
	flag.BoolVar(&printVersion, "version", false, "Prints the current version of consul-dataplane.")

	flag.StringVar(&addresses, "addresses", "", "Consul server addresses. Value can be:\n"+
		"1. DNS name (that resolves to servers or DNS name of a load-balancer front of Consul servers); OR\n"+
		"2.'exec=<executable with optional args>'. The executable\n"+
		"	a) on success - should exit 0 and print to stdout whitespace delimited IP (v4/v6) addresses\n"+
		"	b) on failure - exit with a non-zero code and optionally print an error message of upto 1024 bytes to stderr.\n"+
		"	Refer to https://github.com/hashicorp/go-netaddrs#summary for more details and examples.")

	flag.IntVar(&grpcPort, "grpc-port", 8502, "gRPC port on Consul servers.")

	flag.BoolVar(&serverWatchDisabled, "server-watch-disabled", false, "Setting this prevents consul-dataplane from consuming the server update stream. This is useful for situations where Consul servers are behind a load balancer.")

	flag.StringVar(&logLevel, "log-level", "info", "Log level of the messages to print. "+
		"Available log levels are \"trace\", \"debug\", \"info\", \"warn\", and \"error\".")

	flag.BoolVar(&logJSON, "log-json", false, "Controls consul-dataplane logging in JSON format. By default this is false.")

	flag.StringVar(&nodeName, "service-node-name", "", "The name of the node to which the proxy service instance is registered.")
	flag.StringVar(&nodeID, "service-node-id", "", "The ID of the node to which the proxy service instance is registered.")
	flag.StringVar(&serviceID, "proxy-service-id", "", "The proxy service instance's ID.")
	flag.StringVar(&namespace, "service-namespace", "", "The Consul Enterprise namespace in which the proxy service instance is registered.")
	flag.StringVar(&partition, "service-partition", "", "The Consul Enterprise partition in which the proxy service instance is registered.")

	flag.StringVar(&credentialType, "credential-type", "", "The type of credentials that will be used to authenticate with Consul servers (static or login).")
	flag.StringVar(&token, "static-token", "", "The ACL token used to authenticate requests to Consul servers (when -credential-type is set to static).")
	flag.StringVar(&loginAuthMethod, "login-auth-method", "", "The auth method that will be used to log in.")
	flag.StringVar(&loginNamespace, "login-namespace", "", "The Consul Enterprise namespace containing the auth method.")
	flag.StringVar(&loginPartition, "login-partition", "", "The Consul Enterprise partition containing the auth method.")
	flag.StringVar(&loginDatacenter, "login-datacenter", "", "The datacenter containing the auth method.")
	flag.StringVar(&loginBearerToken, "login-bearer-token", "", "The bearer token that will be presented to the auth method.")
	flag.StringVar(&loginBearerTokenPath, "login-bearer-token-path", "", "The path to a file containing the bearer token that will be presented to the auth method.")
	flag.Var((*FlagMapValue)(&loginMeta), "login-meta", "An arbitrary set of key/value pairs that will be attached to the ACL token (formatted as key=value, may be given multiple times).")

	flag.BoolVar(&useCentralTelemetryConfig, "telemetry-use-central-config", true, "Controls whether the proxy will apply the central telemetry configuration.")

	flag.StringVar(&adminBindAddr, "envoy-admin-bind-address", "127.0.0.1", "The address on which the Envoy admin server will be available.")
	flag.IntVar(&adminBindPort, "envoy-admin-bind-port", 19000, "The port on which the Envoy admin server will be available.")
	flag.StringVar(&readyBindAddr, "envoy-ready-bind-address", "", "The address on which Envoy's readiness probe will be available.")
	flag.IntVar(&readyBindPort, "envoy-ready-bind-port", 0, "The port on which Envoy's readiness probe will be available.")
	flag.IntVar(&envoyConcurrency, "envoy-concurrency", 2, "The envoy concurrency denotes the number of threads that envoy will use https://www.envoyproxy.io/docs/envoy/latest/operations/cli#cmdoption-concurrency.")

	flag.StringVar(&xdsBindAddr, "xds-bind-addr", "127.0.0.1", "The address on which the Envoy xDS server will be available.")
	flag.IntVar(&xdsBindPort, "xds-bind-port", 0, "The port on which the Envoy xDS server will be available.")

	flag.BoolVar(&tlsDisabled, "tls-disabled", false, "Communicate with Consul servers over a plaintext connection. Useful for testing, but not recommended for production.")
	flag.StringVar(&tlsCACertsPath, "ca-certs", "", "The path to a file or directory containing CA certificates that will be used to verify the server's certificate.")
	flag.StringVar(&tlsCertFile, "tls-cert", "", "The path to a client certificate file (only required if tls.grpc.verify_incoming is enabled on the server).")
	flag.StringVar(&tlsKeyFile, "tls-key", "", "The path to a client private key file (only required if tls.grpc.verify_incoming is enabled on the server).")
	flag.StringVar(&tlsServerName, "tls-server-name", "", "The hostname to expect in the server certificate's subject (required if -addresses isn't a DNS name).")
	flag.BoolVar(&tlsInsecureSkipVerify, "tls-insecure-skip-verify", false, "Do not verify the server's certificate. Useful for testing, but not recommended for production.")

	flag.StringVar(&consulDNSBindAddr, "consul-dns-bind-addr", "127.0.0.1", "The address that will be bound to the consul dns proxy.")
	flag.IntVar(&consulDNSPort, "consul-dns-port", -1, "The port the consul dns proxy will listen on. By default 0 disables the dns proxy")

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
