// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/adamthesax/grpc-proxy/proxy"
	"github.com/armon/go-metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	metadataKeyToken   = "x-consul-token"
	envoyADSMethodName = "envoy.service.discovery.v3.AggregatedDiscoveryService/DeltaAggregatedResources"
)

// director is the helper called by the unknown service gRPC handler. This helper is responsible for injecting the ACL token
// into the outgoing Consul server request and returning the target consul server gRPC connection.
func (cdp *ConsulDataplane) director(ctx context.Context, fullMethodName string) (context.Context, *grpc.ClientConn, error) {
	// check to ensure other unknown/unregistered RPCs are not proxied to the target consul server.
	if !strings.Contains(fullMethodName, envoyADSMethodName) {
		return ctx, nil, status.Errorf(codes.Unimplemented, fmt.Sprintf("Unknown method %s", fullMethodName))
	}

	var mdCopy metadata.MD
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		mdCopy = metadata.MD{}
	} else {
		mdCopy = md.Copy()
	}
	mdCopy.Set(metadataKeyToken, cdp.aclToken)
	outCtx := metadata.NewOutgoingContext(ctx, mdCopy)
	return outCtx, cdp.serverConn, nil
}

// setupXDSServer sets up the consul-dataplane xDS server
func (cdp *ConsulDataplane) setupXDSServer() error {
	cdp.logger.Trace("setting up envoy xDS server")

	// create listener to accept envoy xDS connections
	var network, address string
	if strings.HasPrefix(cdp.cfg.XDSServer.BindAddress, "unix://") {
		network = "unix"
		address = cdp.cfg.XDSServer.BindAddress[len("unix://"):]
	} else {
		network = "tcp"
		address = fmt.Sprintf("%s:%d", cdp.cfg.XDSServer.BindAddress, cdp.cfg.XDSServer.BindPort)
	}

	lis, err := net.Listen(network, address)
	if err != nil {
		cdp.logger.Error("failed to create envoy xDS listener: %v", err)
		return err
	}

	// create gRPC server to serve envoy gRPC xDS requests
	// one main role of this gRPC server in consul-dataplane is to proxy envoy ADS requests
	// to the connected Consul server.

	// Note on the underlying library:
	// It has most of the scaffolding to proxy grpc requests to a desired target.
	// The main proxy logic is here - https://github.com/adamthesax/grpc-proxy/blob/master/proxy/handler.go
	// The core library being used is actually this - https://github.com/mwitkow/grpc-proxy.
	// However, we needed this fix (https://github.com/mwitkow/grpc-proxy/pull/62) which was available on the fork we are using.
	// TODO: Switch to the main library once the fix is merged to keep upto date.
	newGRPCServer := grpc.NewServer(
		grpc.UnknownServiceHandler(proxy.TransparentHandler(cdp.director)),
		grpc.StreamInterceptor(cdp.streamInterceptor()),
	)

	cdp.xdsServer = &xdsServer{
		listener:        lis,
		listenerAddress: lis.Addr().String(),
		listenerNetwork: lis.Addr().Network(),
		gRPCServer:      newGRPCServer,
		exitedCh:        make(chan struct{}),
	}

	cdp.logger.Trace("created xDS server", "address", lis.Addr().String())
	return nil
}

func (cdp *ConsulDataplane) startXDSServer(ctx context.Context) {
	cdp.logger.Info("starting envoy xDS server", "address", cdp.xdsServer.listener.Addr().String())

	go func() {
		<-ctx.Done()
		cdp.logger.Info("context done stopping xds server")
		cdp.stopXDSServer()
	}()

	if err := cdp.xdsServer.gRPCServer.Serve(cdp.xdsServer.listener); err != nil {
		cdp.logger.Error("failed to serve xDS requests", "error", err)
		close(cdp.xdsServer.exitedCh)
	}
}

func (cdp *ConsulDataplane) stopXDSServer() {
	if cdp.xdsServer.gRPCServer != nil {
		cdp.logger.Debug("stopping xDS server")
		cdp.xdsServer.gRPCServer.GracefulStop()
	}
}

func (cdp *ConsulDataplane) xdsServerExited() chan struct{} { return cdp.xdsServer.exitedCh }

func (cdp *ConsulDataplane) streamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, &metricServerStream{ss})
	}
}

type metricServerStream struct {
	grpc.ServerStream
}

func (s *metricServerStream) SendMsg(m interface{}) error {
	err := s.ServerStream.SendMsg(m)
	if err == nil {
		metrics.SetGauge([]string{"envoy_connected"}, 1)
		return nil
	}
	metrics.SetGauge([]string{"envoy_connected"}, 0)
	return err
}

func (s *metricServerStream) RecvMsg(m interface{}) error {
	err := s.ServerStream.RecvMsg(m)
	if err == nil {
		metrics.SetGauge([]string{"envoy_connected"}, 1)
		return nil
	}
	metrics.SetGauge([]string{"envoy_connected"}, 0)
	return err
}
