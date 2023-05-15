// Copyright (c) HashiCorp, envoyAdminPort.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	// "bytes"
	"context"
	// "errors"
	"fmt"
	// "io"
	"log"
	// "net"
	"net/http"
	// "strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/consul-dataplane/internal/bootstrap"
	"github.com/stretchr/testify/require"
)

var (
	envoyAdminPort   = 19000
	envoyAdminAddr   = "127.0.0.1"
	envoyShutdownUrl = fmt.Sprintf("http://%s:%v/quitquitquit", envoyAdminAddr, envoyAdminPort)
)

func TestLifecycleServerClosed(t *testing.T) {
	m := &lifecycleConfig{
		mu:                 sync.Mutex{},
		envoyAdminAddr:     envoyAdminAddr,
		envoyAdminBindPort: envoyAdminPort,
		errorExitCh:        make(chan struct{}),

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
		shutdownDrainListeners string
		shutdownGracePeriod    int
		gracefulShutdownPath   string
		gracefulPort           int
	}{
		"connection draining disabled without grace period": {},
		"connection draining enabled without grace period":  {
			// TODO: long-timeout connection held open
		},
		"connection draining disabled with grace period": {},
		"connection draining enabled with grace period":  {
			// TODO: decide if grace period should be a minimum time to wait before
			// shutdown even if all connections have drained, and/or a maximum time
			// even if some connections are still open, test both
		},
		"custom graceful path": {},
		"custom graceful port": {},
	}
	for name, c := range cases {
		c := c
		log.Printf("config = %v", c)

		t.Run(name, func(t *testing.T) {

			m := &lifecycleConfig{
				mu:                 sync.Mutex{},
				envoyAdminAddr:     envoyAdminAddr,
				envoyAdminBindPort: envoyAdminPort,
				errorExitCh:        make(chan struct{}),

				client: &http.Client{
					Timeout: 10 * time.Second,
				},
			}

			require.NotNil(t, m)
			require.NotNil(t, m.client)
			require.NotNil(t, m.errorExitCh)
			require.IsType(t, &http.Client{}, m.client)
			require.Greater(t, m.client.(*http.Client).Timeout, time.Duration(0))

			// Mock get requests to Envoy and Service instance metrics
			// so that they return a fake metric string.
			m.client = &mockClient{}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := m.startLifecycleManager(ctx, &bootstrap.BootstrapConfig{})
			require.NoError(t, err)
			// require.Equal(t, c.bindAddr, m.promScrapeServer.Addr)

			// Have consul-dataplane's metrics server start on an open port.
			// And figure out what port was used so we can make requests to it.
			// Conveniently, this seems to wait until the server is ready for requests.
			/*
				portCh := make(chan int, 1)
				m.promScrapeServer.Addr = "127.0.0.1:0"
				m.promScrapeServer.BaseContext = func(l net.Listener) context.Context {
					portCh <- l.Addr().(*net.TCPAddr).Port
					return context.Background()
				}

				var port int
				select {
				case port = <-portCh:
				case <-time.After(5 * time.Second):
				}

				require.NotEqual(t, port, 0, "test failed to figure out metrics server port")
				log.Printf("port = %v", port)

				url := fmt.Sprintf("http://127.0.0.1:%d/stats/prometheus", port)
				resp, err := http.Get(url)
				require.NoError(t, err)
				require.NotNil(t, resp)

				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
			*/
		})
	}
}
