// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package helpers

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

var httpClient = &http.Client{
	Timeout: 1 * time.Second,
}

// DNSRcodeSuccess mirrors github.com/miekg/dns.RcodeSuccess so callers in the
// test package can assert on it without importing the dns package directly.
const (
	DNSRcodeSuccess = dns.RcodeSuccess
)

func TCP(n int) nat.Port {
	port, err := nat.NewPort("tcp", strconv.Itoa(n))
	if err != nil {
		panic(err)
	}
	return port
}

func UDP(n int) nat.Port {
	port, err := nat.NewPort("udp", strconv.Itoa(n))
	if err != nil {
		panic(err)
	}
	return port
}

func ExpectNoHTTPAccess(t *testing.T, ip string, port int) {
	t.Helper()

	require.Eventually(t, func() bool {
		ok, _ := canAccess(ip, port)
		return !ok
	}, time.Minute, 1*time.Second)
}

func ExpectHTTPAccess(t *testing.T, ip string, port int) {
	t.Helper()

	require.Eventually(t, func() bool {
		ok, err := canAccess(ip, port)
		if err != nil {
			t.Logf("HTTP access check failed: %v\n", err)
		}
		return ok
	}, time.Minute, 1*time.Second)
}

func canAccess(ip string, port int) (bool, error) {
	url := fmt.Sprintf("http://%s/", net.JoinHostPort(ip, strconv.Itoa(port)))
	rsp, err := httpClient.Get(url)
	if err != nil {
		return false, err
	}
	defer rsp.Body.Close()

	if rsp.StatusCode == http.StatusOK {
		return true, nil
	}

	bytes, err := io.ReadAll(rsp.Body)
	return false, fmt.Errorf("unexpected response status: %d - body: %s", rsp.StatusCode, bytes)
}

func DNSLookup(t *testing.T, suite *Suite, protocol string, serverIP string, serverPort int, host string) []string {
	t.Helper()

	ctx, cancel := context.WithTimeout(suite.Context(t), 1*time.Second)
	defer cancel()

	req := new(dns.Msg)
	req.SetQuestion(host, dns.TypeA)

	c := new(dns.Client)
	c.Net = protocol
	rsp, _, err := c.ExchangeContext(
		ctx,
		req,
		net.JoinHostPort(serverIP, strconv.Itoa(serverPort)),
	)
	require.NoError(t, err)

	results := make([]string, len(rsp.Answer))
	for idx, rr := range rsp.Answer {
		results[idx] = rr.(*dns.A).A.String()
	}
	return results
}

// DNSLookupA performs an A-record lookup like DNSLookup but returns the raw
// answer addresses along with the response rcode, so callers can assert on both
// the resolved addresses and the response code. This is used by the
// virtual-domain (VIP) e2e assertions.
func DNSLookupA(t *testing.T, suite *Suite, protocol, serverIP string, serverPort int, host string) (addrs []string, rcode int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(suite.Context(t), 2*time.Second)
	defer cancel()

	req := new(dns.Msg)
	req.SetQuestion(host, dns.TypeA)

	c := new(dns.Client)
	c.Net = protocol
	rsp, _, err := c.ExchangeContext(
		ctx,
		req,
		net.JoinHostPort(serverIP, strconv.Itoa(serverPort)),
	)
	require.NoError(t, err)

	for _, rr := range rsp.Answer {
		if a, ok := rr.(*dns.A); ok {
			addrs = append(addrs, a.A.String())
		}
	}
	return addrs, rsp.Rcode
}

// IsConsulVIP reports whether ip falls in the 240.0.0.0/4 reserved range that
// Consul allocates service virtual IPs (VIPs) from. Virtual-domain
// (*.virtual.consul) queries resolved via Envoy's inline DNS table must return
// an address from this block.
func IsConsulVIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	parsed = parsed.To4()
	if parsed == nil {
		return false
	}
	// 240.0.0.0/4 => first octet in [240, 255].
	return parsed[0] >= 240
}

func GetMetrics(t *testing.T, ip string, port int) string {
	t.Helper()

	url := fmt.Sprintf("http://%s/metrics", net.JoinHostPort(ip, strconv.Itoa(port)))

	rsp, err := httpClient.Get(url)
	require.NoError(t, err)
	defer rsp.Body.Close()

	bytes, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)

	if rsp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected response status: %d - body: %s", rsp.StatusCode, bytes)
	}

	return string(bytes)

}

func GetEnvoyClusters(t *testing.T, ip string, port int) {
	t.Helper()

	url := fmt.Sprintf("http://%s/clusters", net.JoinHostPort(ip, strconv.Itoa(port)))

	_, err := httpClient.Get(url)
	require.NoError(t, err)
}

// EnvoyHasListener reports whether Envoy has a listener whose name contains
// nameSubstr, as reported by the admin /listeners endpoint. The DNS inline and
// egress listeners are provisioned onto Envoy by the Consul server over xDS, so
// their presence here confirms the server pushed them to this proxy.
func EnvoyHasListener(t *testing.T, ip string, port int, nameSubstr string) bool {
	t.Helper()

	url := fmt.Sprintf("http://%s/listeners", net.JoinHostPort(ip, strconv.Itoa(port)))

	rsp, err := httpClient.Get(url)
	require.NoError(t, err)
	defer rsp.Body.Close()

	body, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)

	if rsp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected response status: %d - body: %s", rsp.StatusCode, body)
	}

	return strings.Contains(string(body), nameSubstr)
}
