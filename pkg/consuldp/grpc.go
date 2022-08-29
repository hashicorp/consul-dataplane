package consuldp

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/adamthesax/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const metadataKeyToken = "x-consul-token"

func (cdp *ConsulDataplane) setupGRPCServer() error {
	cdp.logger.Trace("setting up gRPC server")
	// create gRPC listener
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cdp.logger.Error("failed to create gRPC/TCP listener: %v", err)
		return err
	}
	cdp.gRPCListener = lis

	// create gRPC server
	// one main role of this gRPC server in consul-dataplane is to proxy envoy ADS requests
	// to the connected Consul server.
	director := func(ctx context.Context, fullMethodName string) (context.Context, *grpc.ClientConn, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		mdCopy := md.Copy()
		// TODO (NET-148): Inject the ACL token acquired from the server discovery library
		mdCopy[metadataKeyToken] = []string{cdp.cfg.Consul.Credentials.Static.Token}
		outCtx := metadata.NewOutgoingContext(ctx, mdCopy)
		if !strings.Contains(fullMethodName, "envoy.service.discovery.v3.AggregatedDiscoveryService/DeltaAggregatedResources") {
			return outCtx, nil, status.Errorf(codes.Unimplemented, fmt.Sprintf("Unknown method %s", fullMethodName))
		}
		// TODO (NET-148): Ensure the server connection here is the one acquired via the server discovery library
		return outCtx, cdp.consulServer.grpcClientConn, nil
	}
	gRPCServer := grpc.NewServer(grpc.UnknownServiceHandler(proxy.TransparentHandler(director)))
	cdp.gRPCServer = gRPCServer

	cdp.logger.Info("created gRPC server", "address", lis.Addr().String())
	return nil
}

func (cdp *ConsulDataplane) startGRPCServer() {
	cdp.logger.Trace("starting gRPC server")

	if err := cdp.gRPCServer.Serve(cdp.gRPCListener); err != nil {
		cdp.logger.Error("failed to serve gRPC requests: %v", err)
		cdp.gRPCListener.Close()
		// TODO: gracefully exit
	}
}
