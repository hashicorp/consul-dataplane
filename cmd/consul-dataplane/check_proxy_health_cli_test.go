// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

const checkProxyHealthArgsEnv = "TEST_CHECK_PROXY_HEALTH_PORT"

// TestCheckProxyHealthCLIIPv6 runs the real -check-proxy-health code path in a
// subprocess against an Envoy admin stand-in that only listens on [::1].
func TestCheckProxyHealthCLIIPv6(t *testing.T) {
	if port := os.Getenv(checkProxyHealthArgsEnv); port != "" {
		os.Args = []string{"consul-dataplane", "-check-proxy-health",
			"-envoy-admin-bind-address=::1", "-envoy-admin-bind-port=" + port}
		main()
		return
	}

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

	cmd := exec.Command(os.Args[0], "-test.run=^TestCheckProxyHealthCLIIPv6$")
	cmd.Env = append(os.Environ(), checkProxyHealthArgsEnv+"="+strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("-check-proxy-health failed against an Envoy admin server on [::1]: %v\n%s", err, out)
	}
}
