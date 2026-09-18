// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/hashicorp/consul-dataplane/internal/pbdataplane"
)

func TestStartCredentialBrokerDisabled(t *testing.T) {
	cdp, err := NewConsulDP(validConfig(ModeTypeSidecar))
	require.NoError(t, err)
	require.NoError(t, cdp.startCredentialBroker(context.Background()))
}

func TestStartCredentialBrokerRequiresConn(t *testing.T) {
	cfg := validConfig(ModeTypeSidecar)
	cfg.CredentialBroker = &CredentialBrokerConfig{BindAddr: "unix:///tmp/cb.sock"}
	cdp, err := NewConsulDP(cfg)
	require.NoError(t, err)
	err = cdp.startCredentialBroker(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "gRPC connection")
}

func TestStartCredentialBrokerRejectsTCPBind(t *testing.T) {
	cfg := validConfig(ModeTypeSidecar)
	cfg.CredentialBroker = &CredentialBrokerConfig{BindAddr: "127.0.0.1:0"}
	cdp, err := NewConsulDP(cfg)
	require.NoError(t, err)
	cdp.serverConn = new(grpc.ClientConn)
	err = cdp.startCredentialBroker(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "unix socket")
}

type stubKeyClient struct {
	last *pbdataplane.FetchKeyRequest
}

func (s *stubKeyClient) FetchKey(_ context.Context, in *pbdataplane.FetchKeyRequest, _ ...grpc.CallOption) (*pbdataplane.FetchKeyResponse, error) {
	s.last = in
	return &pbdataplane.FetchKeyResponse{KeyId: in.GetKeyId(), KeyMaterial: []byte("dek")}, nil
}

func TestDataplaneKeyFetcherCopiesProxyIdentity(t *testing.T) {
	stub := &stubKeyClient{}
	f := dataplaneKeyFetcher{
		client: stub,
		proxy: &ProxyConfig{
			ProxyID:   "proxy-1",
			NodeName:  "node",
			NodeID:    "nid",
			Namespace: "ns",
			Partition: "part",
		},
	}
	resp, err := f.FetchKey(context.Background(), "k43")
	require.NoError(t, err)
	require.Equal(t, "k43", resp.KeyId)
	require.Equal(t, "proxy-1", stub.last.ProxyId)
	require.Equal(t, "node", stub.last.NodeName)
	require.Equal(t, "nid", stub.last.NodeId)
	require.Equal(t, "ns", stub.last.Namespace)
	require.Equal(t, "part", stub.last.Partition)
}

func TestDataplaneKeyFetcherNilProxy(t *testing.T) {
	stub := &stubKeyClient{}
	f := dataplaneKeyFetcher{client: stub}
	_, err := f.FetchKey(context.Background(), "k43")
	require.NoError(t, err)
	require.Equal(t, "k43", stub.last.KeyId)
	require.Empty(t, stub.last.ProxyId)
}
