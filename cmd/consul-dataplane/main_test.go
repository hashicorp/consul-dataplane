package main

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
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
			statusCode:     200,
			expectedExit:   0,
			expectedOutput: "Envoy proxy is ready.\n",
		},
		{
			name:           "success with 299",
			statusCode:     299,
			expectedExit:   0,
			expectedOutput: "Envoy proxy is ready.\n",
		},
		{
			name:           "failure with 404",
			statusCode:     404,
			expectedExit:   1,
			expectedOutput: "Envoy proxy is not ready. Received status code: 404\n",
		},
		{
			name:           "failure with 500",
			statusCode:     500,
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
			port, _ := strconv.Atoi(u.Port())

			// Capture stdout/stderr
			stdout := captureOutput(t, func() {
				doHealthCheck(port, client, mockExit)
			})

			require.Contains(t, stdout, tc.expectedOutput)
			require.Equal(t, tc.expectedExit, exitCode)

		})
	}
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
	io.Copy(&buf, r)
	return buf.String()
}
