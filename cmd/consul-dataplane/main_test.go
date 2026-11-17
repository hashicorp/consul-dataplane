// Copyright IBM Corp. 2022, 2026

package main

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestDoHealthCheck(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		serverErr      bool
		expectedExit   int
		expectedOutput string
	}{
		{
			name:           "success with 200",
			statusCode:     http.StatusOK,
			expectedExit:   0,
			expectedOutput: "Envoy proxy is ready.\n",
		},
		{
			name:           "success with 204",
			statusCode:     http.StatusNoContent,
			expectedExit:   0,
			expectedOutput: "Envoy proxy is ready.\n",
		},
		{
			name:           "failure with 404",
			statusCode:     http.StatusNotFound,
			expectedExit:   1,
			expectedOutput: "Envoy proxy is not ready. Received status code: 404\n",
		},
		{
			name:           "failure with 500",
			statusCode:     http.StatusInternalServerError,
			expectedExit:   1,
			expectedOutput: "Envoy proxy is not ready. Received status code: 500\n",
		},
		{
			name:           "server error",
			serverErr:      true,
			expectedExit:   1,
			expectedOutput: "Error connecting to Envoy admin endpoint: ",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var exitCode int
			mockExit := func(code int) {
				exitCode = code
			}

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/ready", r.URL.Path)
				require.Equal(t, "GET", r.Method)

				if tc.serverErr {
					panic("simulated server error")
				}

				w.WriteHeader(tc.statusCode)
			}))
			defer ts.Close()

			client := ts.Client()
			u, _ := url.Parse(ts.URL)

			// Capture stdout/stderr
			stdout := captureOutput(t, func() {
				doHealthCheck(u.Host, client, mockExit)
			})

			require.Contains(t, stdout, tc.expectedOutput)
			require.Equal(t, tc.expectedExit, exitCode)

		})
	}
}

func TestEnvoyAdminAddr(t *testing.T) {
	writeConfig := func(t *testing.T, data string) string {
		path := filepath.Join(t.TempDir(), "config.json")
		require.NoError(t, os.WriteFile(path, []byte(data), 0600))
		return path
	}

	tests := []struct {
		name       string
		flags      func(f *FlagOpts)
		configFile string
		expected   string
	}{
		{
			name:     "defaults",
			expected: "127.0.0.1:19000",
		},
		{
			name: "flags",
			flags: func(f *FlagOpts) {
				f.dataplaneConfig.Envoy.AdminBindAddr = strReference("127.0.0.2")
				f.dataplaneConfig.Envoy.AdminBindPort = intReference(19001)
			},
			expected: "127.0.0.2:19001",
		},
		{
			name: "IPv6 bind address",
			flags: func(f *FlagOpts) {
				f.dataplaneConfig.Envoy.AdminBindAddr = strReference("::1")
			},
			expected: "[::1]:19000",
		},
		{
			name:       "config file",
			configFile: `{"envoy": {"adminBindAddress": "::1", "adminBindPort": 19002}}`,
			expected:   "[::1]:19002",
		},
		{
			name:       "flags override config file",
			configFile: `{"envoy": {"adminBindAddress": "::1", "adminBindPort": 19002}}`,
			flags: func(f *FlagOpts) {
				f.dataplaneConfig.Envoy.AdminBindPort = intReference(19003)
			},
			expected: "[::1]:19003",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &FlagOpts{}
			if tc.configFile != "" {
				f.configFile = writeConfig(t, tc.configFile)
			}
			if tc.flags != nil {
				tc.flags(f)
			}

			addr, err := f.envoyAdminAddr()
			require.NoError(t, err)
			require.Equal(t, tc.expected, addr)
		})
	}
}

// TestCheckProxyHealthIPv6 checks that -check-proxy-health reaches an Envoy
// admin server that only listens on the IPv6 loopback address.
func TestCheckProxyHealthIPv6(t *testing.T) {
	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback not available: %v", err)
	}
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts.Listener = ln
	ts.Start()
	defer ts.Close()

	f := &FlagOpts{}
	f.dataplaneConfig.Envoy.AdminBindAddr = strReference("::1")
	f.dataplaneConfig.Envoy.AdminBindPort = intReference(ln.Addr().(*net.TCPAddr).Port)

	addr, err := f.envoyAdminAddr()
	require.NoError(t, err)

	exitCode := -1
	out := captureOutput(t, func() {
		doHealthCheck(addr, ts.Client(), func(code int) { exitCode = code })
	})
	require.Equal(t, 0, exitCode, out)
}

func captureOutput(t *testing.T, f func()) string {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w

	f()

	w.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to copy output: %v", err)
	}
	return buf.String()
}
