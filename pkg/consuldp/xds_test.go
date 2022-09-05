package consuldp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	testToken               = "test-token"
	additionalTestMetaKey   = "additional-meta-key"
	additionalTestMetaValue = "additional-meta-value"
)

func TestDirector(t *testing.T) {
	type testCase struct {
		name            string
		incomingContext context.Context
		methodName      string
		expectedErr     error
	}

	incomingMetadata := metadata.MD{}
	incomingMetadata[additionalTestMetaKey] = []string{additionalTestMetaValue}

	testCases := []testCase{
		{
			name:            "empty metdata in incoming ctx",
			incomingContext: context.Background(),
			methodName:      envoyADSMethodName,
		},
		{
			name:            "non-empty metdata in incoming ctx",
			incomingContext: metadata.NewIncomingContext(context.Background(), incomingMetadata),
			methodName:      envoyADSMethodName,
		},
		{
			name:            "invalid method name",
			incomingContext: metadata.NewIncomingContext(context.Background(), incomingMetadata),
			methodName:      "unknownrpcmethod",
			expectedErr:     status.Errorf(codes.Unimplemented, "Unknown method unknownrpcmethod"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cdp := &ConsulDataplane{
				cfg:          &Config{Consul: &ConsulConfig{Credentials: &CredentialsConfig{Static: &StaticCredentialsConfig{Token: testToken}}}},
				consulServer: &consulServer{grpcClientConn: &grpc.ClientConn{}},
			}
			outctx, targetConn, err := cdp.director(tc.incomingContext, tc.methodName)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, cdp.consulServer.grpcClientConn, targetConn)
				outMD, ok := metadata.FromOutgoingContext(outctx)
				require.True(t, ok)
				require.Equal(t, []string{testToken}, outMD.Get(metadataKeyToken))
				// validate additional metadata in the incoming context is forwarded
				if _, ok := metadata.FromIncomingContext(tc.incomingContext); ok {
					require.Equal(t, []string{additionalTestMetaValue}, outMD.Get(additionalTestMetaKey))
				}
			}
		})
	}
}

func TestSetupXDSServer(t *testing.T) {
	type testCase struct {
		name                    string
		xdsBindAddress          string
		xdsBindPort             int
		expectedListenerNetwork string
		expectedListenerAddress string
	}

	dir := os.TempDir()
	unixSocketPath := filepath.Join(dir, fmt.Sprintf("%d.sock", time.Now().UnixNano()))
	defer func() {
		os.Remove(unixSocketPath)
	}()

	testCases := []testCase{
		{name: "localhost with no port", xdsBindAddress: "127.0.0.1", expectedListenerNetwork: "tcp", expectedListenerAddress: "127.0.0.1"},
		{name: "localhost with port", xdsBindAddress: "127.0.0.1", xdsBindPort: 51804, expectedListenerNetwork: "tcp", expectedListenerAddress: "127.0.0.1:51804"},
		{name: "unix socket", xdsBindAddress: fmt.Sprintf("unix://%s", unixSocketPath), expectedListenerNetwork: "unix", expectedListenerAddress: unixSocketPath},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cdp := &ConsulDataplane{
				cfg:            &Config{XDSServer: &XDSServer{BindAddress: tc.xdsBindAddress, BindPort: tc.xdsBindPort}},
				logger:         hclog.NewNullLogger(),
				localXDSServer: &localXDSServer{enabled: true},
			}

			err := cdp.setupXDSServer()

			require.NoError(t, err)
			require.NotNil(t, cdp.localXDSServer.listener)
			t.Cleanup(func() { cdp.localXDSServer.listener.Close() })
			require.NotNil(t, cdp.localXDSServer.gRPCServer)
			require.Equal(t, tc.expectedListenerNetwork, cdp.localXDSServer.listenerNetwork)
			require.Contains(t, cdp.localXDSServer.listenerAddress, tc.expectedListenerAddress)
			if tc.expectedListenerNetwork == "tcp" && tc.xdsBindPort == 0 {
				listenerPort := cdp.localXDSServer.listenerAddress[len(tc.xdsBindAddress)+1:]
				_, err = strconv.Atoi(listenerPort)
				require.NoError(t, err)
			}
		})
	}

}
