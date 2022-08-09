package main

import (
	"context"
	"flag"
	"log"
	"strings"

	"github.com/hashicorp/consul-dataplane/pkg/consuldp"
)

var (
	addresses string
	grpcPort  int
	logLevel  string
	logJSON   bool
)

func init() {
	flag.StringVar(&addresses, "addresses", "", "Consul server addresses. Value can be:\n"+
		"1. DNS name (that resolves to servers or DNS name of a load-balancer front of Consul servers); OR\n"+
		"2.'exec=<executable with optional args>'. The executable\n"+
		"	a) on success - should exit 0 and print to stdout whitespace delimited IP (v4/v6) addresses\n"+
		"	b) on failure - exit with a non-zero code and optionally print an error message of upto 1024 bytes to stderr.\n"+
		"	Refer to https://github.com/hashicorp/go-netaddrs#summary for more details and examples.")

	flag.IntVar(&grpcPort, "grpc-port", 8502, "gRPC port on Consul servers")

	flag.StringVar(&logLevel, "log-level", "info", "Log level of the messages to print. "+
		"Available log levels are \"trace\", \"debug\", \"info\", \"warn\", and \"error\".")

	flag.BoolVar(&logJSON, "log-json", false, "Controls consul-dataplane logging in JSON format. By default this is false.")
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
		Consul: &consuldp.ConsulConfig{Addresses: addresses, GRPCPort: grpcPort},
		Logging: &consuldp.LoggingConfig{
			Name:     "consul-dataplane",
			LogLevel: strings.ToUpper(logLevel),
			LogJSON:  logJSON,
		},
	}
	consuldpInstance, err := consuldp.NewConsulDP(consuldpCfg)
	if err != nil {
		log.Fatal(err)
	}
	// TODO: Pass cancellable context
	err = consuldpInstance.Run(context.Background())
	if err != nil {
		log.Fatal(err)
	}
}
