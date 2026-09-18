// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"context"
	"fmt"

	"github.com/hashicorp/consul-dataplane/internal/pbdataplane"
	"github.com/hashicorp/consul-dataplane/pkg/credentialbroker"
)

func (cdp *ConsulDataplane) startCredentialBroker(ctx context.Context) error {
	if cdp.cfg.CredentialBroker == nil || cdp.cfg.CredentialBroker.BindAddr == "" {
		return nil
	}
	if cdp.serverConn == nil {
		return fmt.Errorf("credential broker requires a Consul gRPC connection")
	}
	client := pbdataplane.NewWorkloadKeyServiceClient(cdp.serverConn)
	b := credentialbroker.New(credentialbroker.Config{
		BindAddr:        cdp.cfg.CredentialBroker.BindAddr,
		RefreshFraction: cdp.cfg.CredentialBroker.RefreshFraction,
		Logger:          cdp.logger.Named("credential-broker"),
		Fetcher: dataplaneKeyFetcher{
			client: client,
			proxy:  cdp.cfg.Proxy,
		},
	})
	if err := b.Start(ctx); err != nil {
		return fmt.Errorf("start credential broker: %w", err)
	}
	return nil
}

type dataplaneKeyFetcher struct {
	client pbdataplane.WorkloadKeyServiceClient
	proxy  *ProxyConfig
}

func (f dataplaneKeyFetcher) FetchKey(ctx context.Context, keyID string) (*pbdataplane.FetchKeyResponse, error) {
	req := &pbdataplane.FetchKeyRequest{KeyId: keyID}
	if f.proxy != nil {
		req.ProxyId = f.proxy.ProxyID
		req.NodeName = f.proxy.NodeName
		req.NodeId = f.proxy.NodeID
		req.Namespace = f.proxy.Namespace
		req.Partition = f.proxy.Partition
	}
	return f.client.FetchKey(ctx, req)
}
