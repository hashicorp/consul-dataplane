// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/consul/proto-public/pbdataplane"
	pbmesh "github.com/hashicorp/consul/proto-public/pbmesh/v1alpha1"
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

// bootstrapConfig generates the Envoy bootstrap config in JSON format.
func (cdp *ConsulDataplane) bootstrapConfig(ctx context.Context) (*bootstrap.BootstrapConfig, []byte, error) {
	svc := cdp.cfg.Proxy
	envoy := cdp.cfg.Envoy

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
		return nil, nil, fmt.Errorf("failed to get envoy bootstrap params: %w", err)
	}

	prom := cdp.cfg.Telemetry.Prometheus
	args := &bootstrap.BootstrapTplArgs{
		GRPC: bootstrap.GRPC{
			AgentAddress: cdp.cfg.XDSServer.BindAddress,
			AgentPort:    strconv.Itoa(cdp.cfg.XDSServer.BindPort),
			AgentTLS:     false,
		},
		ProxyCluster:          rsp.Service,
		ProxyID:               svc.ProxyID,
		NodeName:              rsp.NodeName,
		ProxySourceService:    rsp.Service,
		AdminAccessLogConfig:  rsp.AccessLogs,
		AdminAccessLogPath:    defaultAdminAccessLogsPath,
		AdminBindAddress:      envoy.AdminBindAddress,
		AdminBindPort:         strconv.Itoa(envoy.AdminBindPort),
		LocalAgentClusterName: localClusterName,
		Namespace:             rsp.Namespace,
		Partition:             rsp.Partition,
		Datacenter:            rsp.Datacenter,
		PrometheusCertFile:    prom.CertFile,
		PrometheusKeyFile:     prom.KeyFile,
		PrometheusScrapePath:  prom.ScrapePath,
	}

	if rsp.Identity != "" {
		args.ProxyCluster = rsp.Identity
		args.ProxySourceService = rsp.Identity
	}

	if cdp.xdsServer.listenerNetwork == "unix" {
		args.GRPC.AgentSocket = cdp.xdsServer.listenerAddress
	} else {
		xdsServerFullAddr := strings.Split(cdp.xdsServer.listenerAddress, ":")
		args.GRPC.AgentAddress = xdsServerFullAddr[0]
		args.GRPC.AgentPort = xdsServerFullAddr[1]
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
		if rsp.BootstrapConfig != nil {
			bootstrapConfig = bootstrapConfigFromCfg(rsp.BootstrapConfig)
		} else {
			if err := mapstructure.WeakDecode(rsp.Config.AsMap(), &bootstrapConfig); err != nil {
				return nil, nil, fmt.Errorf("failed parsing Proxy.Config: %w", err)
			}
		}

		// Envoy is configured with a listener that proxies metrics from its
		// own admin endpoint (localhost:19000/stats/prometheus). When central
		// config is enabled, we set the PrometheusBackendPort to instead have
		// Envoy proxy metrics from Consul Dataplane which serves merged
		// metrics (Envoy + Dataplane + service metrics).
		// Documentation: https://www.consul.io/commands/connect/envoy#prometheus-backend-port
		args.PrometheusBackendPort = strconv.Itoa(prom.MergePort)
	}

	// Note: we pass true for omitDeprecatedTags here - consul-dataplane is clean
	// slate and we don't need to maintain this legacy behavior.
	cfg, err := bootstrapConfig.GenerateJSON(args, true)
	return &bootstrapConfig, cfg, err
}

func bootstrapConfigFromCfg(cfg *pbmesh.BootstrapConfig) bootstrap.BootstrapConfig {
	return bootstrap.BootstrapConfig{
		StatsdURL:                       cfg.StatsdUrl,
		DogstatsdURL:                    cfg.DogstatsdUrl,
		StatsTags:                       cfg.StatsTags,
		TelemetryCollectorBindSocketDir: cfg.TelemetryCollectorBindSocketDir,
		PrometheusBindAddr:              cfg.PrometheusBindAddr,
		StatsBindAddr:                   cfg.StatsBindAddr,
		ReadyBindAddr:                   cfg.ReadyBindAddr,
		OverrideJSONTpl:                 cfg.OverrideJsonTpl,
		StaticClustersJSON:              cfg.StaticClustersJson,
		StaticListenersJSON:             cfg.StaticListenersJson,
		StatsSinksJSON:                  cfg.StatsSinksJson,
		StatsConfigJSON:                 cfg.StatsConfigJson,
		StatsFlushInterval:              cfg.StatsFlushInterval,
		TracingConfigJSON:               cfg.TracingConfigJson,
	}
}
