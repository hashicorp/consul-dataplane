// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/hashicorp/consul-dataplane/internal/pbdataplane"
	"github.com/hashicorp/consul-dataplane/pkg/credentialbroker"
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

func TestStartCredentialBrokerSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join("/tmp", filepath.Base(dir)+"-cdp-cb.sock")
	t.Cleanup(func() { _ = os.Remove(path) })
	cfg := validConfig(ModeTypeSidecar)
	cfg.CredentialBroker = &CredentialBrokerConfig{BindAddr: path}
	cdp, err := NewConsulDP(cfg)
	require.NoError(t, err)
	cdp.serverConn = new(grpc.ClientConn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, cdp.startCredentialBroker(ctx))
	require.NotNil(t, cdp.credentialBroker)
	require.NotNil(t, cdp.credentialBrokerExited())
}

func TestCredentialBrokerExitedNilWhenDisabled(t *testing.T) {
	cdp, err := NewConsulDP(validConfig(ModeTypeSidecar))
	require.NoError(t, err)
	require.Nil(t, cdp.credentialBrokerExited())
	cdp.credentialBroker = credentialbroker.New(credentialbroker.Config{})
	require.Nil(t, cdp.credentialBrokerExited())
}

type stubProxy struct {
	quitErr error
	killErr error
	killed  bool
}

func (s *stubProxy) Quit() error { return s.quitErr }
func (s *stubProxy) Kill() error { s.killed = true; return s.killErr }

func TestHandleCredentialBrokerExit(t *testing.T) {
	cdp, err := NewConsulDP(validConfig(ModeTypeSidecar))
	require.NoError(t, err)

	err = cdp.handleCredentialBrokerExit(&stubProxy{}, nil)
	require.EqualError(t, err, "credential broker exited unexpectedly")

	err = cdp.handleCredentialBrokerExit(&stubProxy{}, errors.New("serve failed"))
	require.ErrorContains(t, err, "serve failed")

	proxy := &stubProxy{quitErr: errors.New("quit"), killErr: errors.New("kill")}
	err = cdp.handleCredentialBrokerExit(proxy, nil)
	require.EqualError(t, err, "credential broker exited unexpectedly")
	require.True(t, proxy.killed)
}

func TestDataplaneKeyFetcherNilProxy(t *testing.T) {
	stub := &stubKeyClient{}
	f := dataplaneKeyFetcher{client: stub}
	_, err := f.FetchKey(context.Background(), "k43")
	require.NoError(t, err)
	require.Equal(t, "k43", stub.last.KeyId)
	require.Empty(t, stub.last.ProxyId)
}
