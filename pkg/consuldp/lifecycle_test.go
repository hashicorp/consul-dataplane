// Copyright (c) HashiCorp, envoyAdminPort.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	// "bytes"
	"context"
	// "errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	// "strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/consul-dataplane/internal/bootstrap"
	"github.com/stretchr/testify/require"
)

var (
	envoyAdminPort = 19000
	envoyAdminAddr = "127.0.0.1"
)

func TestLifecycleServerClosed(t *testing.T) {
	m := &lifecycleConfig{
		mu:                 sync.Mutex{},
		envoyAdminAddr:     envoyAdminAddr,
		envoyAdminBindPort: envoyAdminPort,
		doneCh:             make(chan struct{}, 1),

		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	_ = m.startLifecycleManager(ctx, &bootstrap.BootstrapConfig{})
	require.Equal(t, m.running, true)
	cancel()
	require.Eventually(t, func() bool {
		return !m.running
	}, time.Second*2, time.Second)

}

func TestLifecycleServerEnabled(t *testing.T) {
	cases := map[string]struct {
		shutdownDrainListeners bool
		shutdownGracePeriod    int
		gracefulShutdownPath   string
		gracefulPort           int
	}{
		// TODO: testing the actual Envoy behavior here such as how open or new
		// connections are handled should happpen in integration or acceptance tests
		"connection draining disabled without grace period": {
			// All inbound and outbound connections are terminated immediately.
		},
		"connection draining enabled without grace period": {
			// This should immediately send "Connection: close" to inbound HTTP1
			// connections, GOAWAY to inbound HTTP2, and terminate connections on
			// request completion. Outbound connections should start being rejected
			// immediately.
			shutdownDrainListeners: true,
		},
		"connection draining disabled with grace period": {
			// This should immediately terminate any open inbound connections.
			// Outbound connections should be allowed until the grace period has
			// elapsed.
			shutdownGracePeriod: 5,
		},
		"connection draining enabled with grace period": {
			// This should immediately send "Connection: close" to inbound HTTP1
			// connections, GOAWAY to inbound HTTP2, and terminate connections on
			// request completion.
			// Outbound connections should be allowed until the grace period has
			// elapsed, then any remaining open connections should be closed and new
			// outbound connections should start being rejected until pod termination.
			shutdownDrainListeners: true,
			shutdownGracePeriod:    5,
		},
		"custom graceful shutdown path and port": {
			shutdownDrainListeners: true,
			shutdownGracePeriod:    5,
			gracefulShutdownPath:   "/quit-nicely",
			// TODO: should this be random or use freeport? logic disallows passing
			// zero value explicitly
			gracefulPort: 23108,
		},
	}
	for name, c := range cases {
		c := c
		log.Printf("config = %v", c)

		t.Run(name, func(t *testing.T) {
			m := &lifecycleConfig{
				envoyAdminAddr:         envoyAdminAddr,
				envoyAdminBindPort:     envoyAdminPort,
				shutdownDrainListeners: c.shutdownDrainListeners,
				shutdownGracePeriod:    c.shutdownGracePeriod,
				gracefulShutdownPath:   c.gracefulShutdownPath,
				gracefulPort:           c.gracefulPort,

				client: &http.Client{
					Timeout: 10 * time.Second,
				},

				doneCh: make(chan struct{}, 1),
				mu:     sync.Mutex{},
			}

			require.NotNil(t, m)
			require.NotNil(t, m.client)
			require.NotNil(t, m.doneCh)
			require.IsType(t, &http.Client{}, m.client)
			require.Greater(t, m.client.(*http.Client).Timeout, time.Duration(0))

			// Mock requests to Envoy so that admin API responses can be controlled
			m.client = &mockClient{}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := m.startLifecycleManager(ctx, &bootstrap.BootstrapConfig{})
			require.NoError(t, err)

			// Have consul-dataplane's lifecycle server start on an open port
			// and figure out what port was used so we can make requests to it.
			// Conveniently, this seems to wait until the server is ready for requests.
			portCh := make(chan int, 1)
			// m.lifecycleServer.Addr = "127.0.0.1:0"
			m.lifecycleServer.BaseContext = func(l net.Listener) context.Context {
				portCh <- l.Addr().(*net.TCPAddr).Port
				return context.Background()
			}

			var port int
			select {
			case port = <-portCh:
			case <-time.After(5 * time.Second):
			}

			// Check lifecycle server graceful port configuration
			if c.gracefulPort != 0 {
				require.Equal(t, port, c.gracefulPort, "failed to set lifecycle server port")
			} else {
				require.Equal(t, port, 20300, "failed to figure out default lifecycle server port")
			}
			log.Printf("port = %v\n", port)

			// Check lifecycle server graceful shutdown path configuration
			if c.gracefulShutdownPath != "" {
				require.Equal(t, m.gracefulShutdownPath, c.gracefulShutdownPath, "failed to set lifecycle server graceful shutdown HTTP endpoint path")
			}

			// Check lifecycle server graceful shutdown path configuration
			url := fmt.Sprintf("http://127.0.0.1:%d%s", port, m.gracefulShutdownPath)
			log.Printf("sending request to %s\n", url)

			resp, err := http.Get(url)

			// TODO: use mock client to check envoyAdminAddr and envoyAdminPort?
			// m.client.Expect(address, port)

			require.NoError(t, err)
			require.NotNil(t, resp)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NotNil(t, body)
		})
	}
}
