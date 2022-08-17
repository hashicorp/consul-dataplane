package consuldp

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/hashicorp/consul-dataplane/internal/consul-proto/pbdataplane"
	"github.com/hashicorp/go-hclog"
	netaddrs "github.com/hashicorp/go-netaddrs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// consulServer maintains the settings of the Consul server with which
// consul-dataplane has established a gRPC connection
type consulServer struct {
	// address is the IP address of the Consul server
	address net.IPAddr
	// supportedFeatures is a map of the dataplane features supported by the Consul server
	supportedFeatures map[pbdataplane.DataplaneFeatures]bool
}

// ConsulDataplane represents the consul-dataplane process
type ConsulDataplane struct {
	logger          hclog.Logger
	cfg             *Config
	consulServer    *consulServer
	dpServiceClient pbdataplane.DataplaneServiceClient
}

// NewConsulDP creates a new instance of ConsulDataplane
func NewConsulDP(cfg *Config) (*ConsulDataplane, error) {
	if cfg.Consul == nil || cfg.Consul.Addresses == "" {
		return nil, fmt.Errorf("consul addresses not specified")
	}
	if cfg.Consul.GRPCPort == 0 {
		return nil, fmt.Errorf("consul server gRPC port not specified")
	}

	hclogLevel := hclog.LevelFromString(cfg.Logging.LogLevel)
	if hclogLevel == hclog.NoLevel {
		hclogLevel = hclog.Info
	}
	logger := hclog.New(&hclog.LoggerOptions{
		Name:       cfg.Logging.Name,
		Level:      hclogLevel,
		JSONFormat: cfg.Logging.LogJSON,
	})

	return &ConsulDataplane{logger: logger, cfg: cfg}, nil
}

// TODO (CSLC-151): Integrate with server discovery library to determine a healthy server for grpc/xds connection
func (cdp *ConsulDataplane) resolveAndPickConsulServerAddress(ctx context.Context) error {
	netAddrLogger := cdp.logger.Named("go-netaddrs")
	addresses, err := netaddrs.IPAddrs(ctx, cdp.cfg.Consul.Addresses, netAddrLogger)
	if err != nil {
		errMsg := "failure resolving consul server addresses"
		cdp.logger.Error(errMsg, "error", err)
		return fmt.Errorf("%s. %v", errMsg, err)
	}
	cdp.logger.Info("resolved consul server addresses", "addresses", addresses)
	// randomly pick a server address from the list of resolved addresses
	rand.Seed(time.Now().Unix())
	cdp.consulServer = &consulServer{address: addresses[rand.Intn(len(addresses))]}
	return nil
}

func (cdp *ConsulDataplane) setConsulServerSupportedFeatures(ctx context.Context) error {
	resp, err := cdp.dpServiceClient.GetSupportedDataplaneFeatures(ctx, &pbdataplane.GetSupportedDataplaneFeaturesRequest{})
	if err != nil {
		errMsg := "failure getting supported consul-dataplane features"
		cdp.logger.Error(errMsg, "error", err)
		return fmt.Errorf("%s. %v", errMsg, err)
	}

	serverSupportedFeatures := make(map[pbdataplane.DataplaneFeatures]bool)
	cdp.logger.Info("retrieved consul server supported dataplane features")
	for _, feature := range resp.SupportedDataplaneFeatures {
		serverSupportedFeatures[feature.GetFeatureName()] = feature.GetSupported()
		cdp.logger.Info("feature support", feature.GetFeatureName().String(), feature.GetSupported())
	}
	cdp.consulServer.supportedFeatures = serverSupportedFeatures
	return nil
}

func (cdp *ConsulDataplane) Run(ctx context.Context) error {
	cdp.logger.Info("started consul-dataplane process")

	if err := cdp.resolveAndPickConsulServerAddress(ctx); err != nil {
		return err
	}

	// Establish gRPC connection to the Consul server
	// TODO: Use TLS for the gRPC connection
	gRPCTarget := fmt.Sprintf("%s:%d", cdp.consulServer.address.String(), cdp.cfg.Consul.GRPCPort)
	grpcCtx, cancel := context.WithTimeout(ctx, time.Duration(10*time.Second))
	defer cancel()
	grpcClientConn, err := grpc.DialContext(grpcCtx, gRPCTarget, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		cdp.logger.Error("could not connect to consul server over grpc", "error", err, "grpc-target", gRPCTarget)
		return err
	}
	defer grpcClientConn.Close()
	cdp.logger.Info("connected to consul server over grpc", "grpc-target", gRPCTarget)

	dpservice := pbdataplane.NewDataplaneServiceClient(grpcClientConn)
	cdp.dpServiceClient = dpservice

	// TODO: Acquire ACL token and pass it in gRPC calls.

	if err := cdp.setConsulServerSupportedFeatures(ctx); err != nil {
		return err
	}

	cfg, err := cdp.bootstrapConfig(ctx)
	if err != nil {
		return err
	}
	cdp.logger.Debug("generated envoy bootstrap config", "config", string(cfg))

	return nil
}
