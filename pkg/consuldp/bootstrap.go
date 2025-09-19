// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/hashicorp/consul/proto-public/pbdataplane"
	"github.com/mitchellh/mapstructure"

	"github.com/hashicorp/consul-dataplane/internal/bootstrap"
)

const (
	// This is the name of the Envoy cluster used to communicate with the
	// dataplane process via xDS.
	localClusterName = "consul-dataplane"

	// By default we send logs from Envoy's admin interface to /dev/null.
	defaultAdminAccessLogsPath = os.DevNull
)

// getBootstrapParams makes a call using the service client to get the bootstrap params for eventually getting the Envoy bootstrap config.
func (cdp *ConsulDataplane) getBootstrapParams(ctx context.Context) (*pbdataplane.GetEnvoyBootstrapParamsResponse, error) {
	svc := cdp.cfg.Proxy

	req := &pbdataplane.GetEnvoyBootstrapParamsRequest{
		ServiceId: svc.ProxyID,
		ProxyId:   svc.ProxyID,
		Namespace: svc.Namespace,
		Partition: svc.Partition,
	}

	if svc.NodeID != "" {
		req.NodeSpec = &pbdataplane.GetEnvoyBootstrapParamsRequest_NodeId{
			NodeId: svc.NodeID,
		}
	} else {
		req.NodeSpec = &pbdataplane.GetEnvoyBootstrapParamsRequest_NodeName{
			NodeName: svc.NodeName,
		}
	}

	rsp, err := cdp.dpServiceClient.GetEnvoyBootstrapParams(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get envoy bootstrap params: %w", err)
	}

	return rsp, nil
}

// bootstrapConfig generates the Envoy bootstrap config in JSON format.
func (cdp *ConsulDataplane) bootstrapConfig(
	bootstrapParams *pbdataplane.GetEnvoyBootstrapParamsResponse) (*bootstrap.BootstrapConfig, []byte, error) {
	svc := cdp.cfg.Proxy
	envoy := cdp.cfg.Envoy

	prom := cdp.cfg.Telemetry.Prometheus
	args := &bootstrap.BootstrapTplArgs{
		GRPC: bootstrap.GRPC{
			AgentAddress: cdp.cfg.XDSServer.BindAddress,
			AgentPort:    strconv.Itoa(cdp.cfg.XDSServer.BindPort),
			AgentTLS:     false,
		},
		ProxyCluster:          bootstrapParams.Service,
		ProxyID:               svc.ProxyID,
		NodeName:              bootstrapParams.NodeName,
		ProxySourceService:    bootstrapParams.Service,
		AdminAccessLogConfig:  bootstrapParams.AccessLogs,
		AdminAccessLogPath:    defaultAdminAccessLogsPath,
		AdminBindAddress:      envoy.AdminBindAddress,
		AdminBindPort:         strconv.Itoa(envoy.AdminBindPort),
		LocalAgentClusterName: localClusterName,
		Namespace:             bootstrapParams.Namespace,
		Partition:             bootstrapParams.Partition,
		Datacenter:            bootstrapParams.Datacenter,
		PrometheusCertFile:    prom.CertFile,
		PrometheusKeyFile:     prom.KeyFile,
		PrometheusScrapePath:  prom.ScrapePath,
	}

	if bootstrapParams.Identity != "" {
		args.ProxyCluster = bootstrapParams.Identity
		args.ProxySourceService = bootstrapParams.Identity
	}

	if cdp.xdsServer.listenerNetwork == "unix" {
		args.AgentSocket = cdp.xdsServer.listenerAddress
	} else {
		h, p, err := net.SplitHostPort(cdp.xdsServer.listenerAddress)
		if err != nil {
			cdp.logger.Error("error splitting listenerAddress to host and port with error", err)
		}
		args.AgentAddress = h
		args.AgentPort = p
	}

	if path := prom.CACertsPath; path != "" {
		fi, err := os.Stat(path)
		if err != nil {
			return nil, nil, err
		}
		if fi.IsDir() {
			args.PrometheusCAPath = path
		} else {
			args.PrometheusCAFile = path
		}
	}

	var bootstrapConfig bootstrap.BootstrapConfig
	if envoy.ReadyBindAddress != "" && envoy.ReadyBindPort != 0 {
		bootstrapConfig.ReadyBindAddr = net.JoinHostPort(envoy.ReadyBindAddress, strconv.Itoa(envoy.ReadyBindPort))
	}

	if cdp.cfg.Telemetry.UseCentralConfig {
		if err := mapstructure.WeakDecode(bootstrapParams.Config.AsMap(), &bootstrapConfig); err != nil {
			return nil, nil, fmt.Errorf("failed parsing Proxy.Config: %w", err)
		}

		// Envoy is configured with a listener that proxies metrics from its
		// own admin endpoint (localhost:19000/stats/prometheus). When central
		// config is enabled, we set the PrometheusBackendPort to instead have
		// Envoy proxy metrics from Consul Dataplane which serves merged
		// metrics (Envoy + Dataplane + service metrics).
		// Documentation: https://www.consul.io/commands/connect/envoy#prometheus-backend-port
		args.PrometheusBackendPort = strconv.Itoa(prom.MergePort)
	}

	bootstrapConfig.Logger = cdp.logger.Named("bootstrap-config")

	// Note: we pass true for omitDeprecatedTags here - consul-dataplane is clean
	// slate, and we don't need to maintain this legacy behavior.
	cfg, err := bootstrapConfig.GenerateJSON(args, true)
	return &bootstrapConfig, cfg, err
}
