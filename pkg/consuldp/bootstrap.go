package consuldp

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/mitchellh/mapstructure"

	"github.com/hashicorp/consul-dataplane/internal/bootstrap"
	"github.com/hashicorp/consul-dataplane/internal/consul-proto/pbdataplane"
)

const (
	// This is the name of the Envoy cluster used to communicate with the
	// dataplane process via xDS.
	localClusterName = "consul-dataplane"

	// By default we send logs from Envoy's admin interface to /dev/null.
	defaultAdminAccessLogsPath = os.DevNull
)

// bootstrapConfig generates the Envoy bootstrap config in JSON format.
func (cdp *ConsulDataplane) bootstrapConfig(ctx context.Context) ([]byte, error) {
	svc := cdp.cfg.Service
	envoy := cdp.cfg.Envoy

	req := &pbdataplane.GetEnvoyBootstrapParamsRequest{
		ServiceId: svc.ServiceID,
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

	args := &bootstrap.BootstrapTplArgs{
		GRPC: bootstrap.GRPC{
			// TODO(NET-99): This should be a listener on the consul-dataplane process
			// that proxies streams to the server, handles load-balancing, SDS etc.
			//
			// For now we just give the server address directly.
			AgentAddress: cdp.consulServer.address.String(),
			AgentPort:    strconv.Itoa(cdp.cfg.Consul.GRPCPort),
			AgentTLS:     false,
		},
		ProxyCluster:          rsp.Service,
		ProxyID:               svc.ServiceID,
		NodeName:              rsp.NodeName,
		ProxySourceService:    rsp.Service,
		AdminAccessLogPath:    defaultAdminAccessLogsPath,
		AdminBindAddress:      envoy.AdminBindAddress,
		AdminBindPort:         strconv.Itoa(envoy.AdminBindPort),
		LocalAgentClusterName: localClusterName,
		// TODO(NET-??): Support login via an ACL auth-method.
		Token:      cdp.cfg.Consul.Credentials.Static.Token,
		Namespace:  rsp.Namespace,
		Partition:  rsp.Partition,
		Datacenter: rsp.Datacenter,
	}

	var bootstrapConfig bootstrap.BootstrapConfig
	if envoy.ReadyBindAddress != "" && envoy.ReadyBindPort != 0 {
		bootstrapConfig.ReadyBindAddr = net.JoinHostPort(envoy.ReadyBindAddress, strconv.Itoa(envoy.ReadyBindPort))
	}

	if cdp.cfg.Telemetry.UseCentralConfig {
		if err := mapstructure.WeakDecode(rsp.Config.AsMap(), &bootstrapConfig); err != nil {
			return nil, fmt.Errorf("failed parsing Proxy.Config: %w", err)
		}
	}

	// Note: we pass true for omitDeprecatedTags here - consul-dataplane is clean
	// slate and we don't need to maintain this legacy behavior.
	return bootstrapConfig.GenerateJSON(args, true)
}
