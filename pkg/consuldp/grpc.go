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

	// create gRPC server
	// one main role of this gRPC server in consul-dataplane is to proxy envoy ADS requests
	// to the connected Consul server.
	director := func(ctx context.Context, fullMethodName string) (context.Context, *grpc.ClientConn, error) {
		if !strings.Contains(fullMethodName, "envoy.service.discovery.v3.AggregatedDiscoveryService/DeltaAggregatedResources") {
			return ctx, nil, status.Errorf(codes.Unimplemented, fmt.Sprintf("Unknown method %s", fullMethodName))
		}

		md, _ := metadata.FromIncomingContext(ctx)
		mdCopy := md.Copy()
		// TODO (NET-148): Inject the ACL token acquired from the server discovery library
		mdCopy[metadataKeyToken] = []string{cdp.cfg.Consul.Credentials.Static.Token}
		outCtx := metadata.NewOutgoingContext(ctx, mdCopy)
		return outCtx, cdp.consulServer.grpcClientConn, nil
	}
	newGRPCServer := grpc.NewServer(grpc.UnknownServiceHandler(proxy.TransparentHandler(director)))

	cdp.gRPCServer = &gRPCServer{listener: lis, server: newGRPCServer, exitedCh: make(chan struct{})}
	cdp.logger.Trace("created gRPC server", "address", lis.Addr().String())
	return nil
}

func (cdp *ConsulDataplane) startGRPCServer() {
	cdp.logger.Info("starting gRPC server", "address", cdp.gRPCServer.listener.Addr().String())

	if err := cdp.gRPCServer.server.Serve(cdp.gRPCServer.listener); err != nil {
		cdp.logger.Error("failed to serve gRPC requests", "error", err)
		cdp.gRPCServer.listener.Close()
		close(cdp.gRPCServer.exitedCh)
	}
}

func (cdp *ConsulDataplane) stopGRPCServer() {
	if cdp.gRPCServer != nil {
		cdp.logger.Debug("stopping gRPC server")
		cdp.gRPCServer.server.Stop()
	}
}

func (cdp *ConsulDataplane) gRPCServerExited() chan struct{} { return cdp.gRPCServer.exitedCh }
