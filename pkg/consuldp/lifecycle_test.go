// Copyright (c) HashiCorp, envoyAdminPort.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var (
	envoyAdminPort = 19000
	envoyAdminAddr = "127.0.0.1"
)

func TestLifecycleServerClosed(t *testing.T) {
	cfg := Config{
		Envoy: &EnvoyConfig{
			AdminBindAddress: envoyAdminAddr,
			AdminBindPort:    envoyAdminPort,
		},
	}
	m := NewLifecycleConfig(&cfg, &mockProxy{})

	ctx, cancel := context.WithCancel(context.Background())

	_ = m.startLifecycleManager(ctx)
	require.Equal(t, m.running, true)
	cancel()
	require.Eventually(t, func() bool {
		return !m.running
	}, time.Second*2, time.Second)

}

func TestLifecycleServerEnabled(t *testing.T) {
	cases := map[string]struct {
		shutdownDrainListenersEnabled bool
		shutdownGracePeriodSeconds    int
		gracefulShutdownPath          string
		gracefulPort                  int
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
			shutdownDrainListenersEnabled: true,
		},
		"connection draining disabled with grace period": {
			// This should immediately terminate any open inbound connections.
			// Outbound connections should be allowed until the grace period has
			// elapsed.
			shutdownGracePeriodSeconds: 5,
		},
		"connection draining enabled with grace period": {
			// This should immediately send "Connection: close" to inbound HTTP1
			// connections, GOAWAY to inbound HTTP2, and terminate connections on
			// request completion.
			// Outbound connections should be allowed until the grace period has
			// elapsed, then any remaining open connections should be closed and new
			// outbound connections should start being rejected until pod termination.
			shutdownDrainListenersEnabled: true,
			shutdownGracePeriodSeconds:    5,
		},
		"custom graceful shutdown path and port": {
			shutdownDrainListenersEnabled: true,
			shutdownGracePeriodSeconds:    5,
			gracefulShutdownPath:          "/quit-nicely",
			// TODO: should this be random or use freeport? logic disallows passing
			// zero value explicitly
			gracefulPort: 23108,
		},
	}
	for name, c := range cases {
		c := c
		log.Printf("config = %v", c)

		t.Run(name, func(t *testing.T) {
			// Add a small margin of error for assertions checking expected
			// behavior within the shutdown grace period window.
			shutdownTimeout := time.Duration((c.shutdownGracePeriodSeconds + 5)) * time.Second

			cfg := Config{
				Envoy: &EnvoyConfig{
					AdminBindAddress:              envoyAdminAddr,
					AdminBindPort:                 envoyAdminPort,
					ShutdownDrainListenersEnabled: c.shutdownDrainListenersEnabled,
					ShutdownGracePeriodSeconds:    c.shutdownGracePeriodSeconds,
					GracefulShutdownPath:          c.gracefulShutdownPath,
					GracefulPort:                  c.gracefulPort,
				},
			}
			m := NewLifecycleConfig(&cfg, &mockProxy{})

			require.NotNil(t, m)
			require.NotNil(t, m.proxy)
			require.NotNil(t, m.errorExitCh)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := m.startLifecycleManager(ctx)
			require.NoError(t, err)

			// Have consul-dataplane's lifecycle server start on an open port
			// and figure out what port was used so we can make requests to it.
			// Conveniently, this seems to wait until the server is ready for requests.
			portCh := make(chan int, 1)
			if c.gracefulPort == 0 {
				m.lifecycleServer.Addr = "127.0.0.1:0"
			}
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
				require.Equal(t, c.gracefulPort, port, "failed to set lifecycle server port")
			} else {
				require.NotEqual(t, 0, port, "failed to figure out lifecycle server port")
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

			// HTTP handler is not blocking, so need to wait and check mock
			// client for expected method calls to proxy manager within
			// expected shutdown grace period plus a small margin of error.
			if c.shutdownDrainListenersEnabled {
				require.Eventually(t, func() bool {
					return m.proxy.(*mockProxy).drainCalled == 1
				}, shutdownTimeout, time.Second, "Proxy.Drain() not called as expected")
			} else {
				require.Never(t, func() bool {
					return m.proxy.(*mockProxy).drainCalled == 1
				}, shutdownTimeout, time.Second, "Proxy.Drain() called unexpectedly")
			}

			require.Eventually(t, func() bool {
				return m.proxy.(*mockProxy).quitCalled == 1
			}, shutdownTimeout, time.Second, "Proxy.Quit() not called as expected")

			// Expect that proxy is not forcefully killed as part of graceful shutdown.
			require.Never(t, func() bool {
				return m.proxy.(*mockProxy).killCalled == 1
			}, shutdownTimeout, time.Second, "Proxy.Kill() called unexpectedly")

			require.NoError(t, err)
			require.NotNil(t, resp)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NotNil(t, body)
		})
	}
}

type mockProxy struct {
	runCalled   int
	drainCalled int
	quitCalled  int
	killCalled  int
}

func (p *mockProxy) Run(ctx context.Context) error {
	p.runCalled++
	return nil
}

func (p *mockProxy) Drain() error {
	p.drainCalled++
	return nil
}

func (p *mockProxy) Quit() error {
	p.quitCalled++
	return nil
}
func (p *mockProxy) Kill() error {
	p.killCalled++
	return nil
}

func (p *mockProxy) DumpConfig() error {
	return nil
}
func (p *mockProxy) Ready() (bool, error) {
	return false, nil
}
