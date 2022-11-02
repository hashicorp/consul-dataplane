package helpers

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

var httpClient = &http.Client{
	Timeout: 1 * time.Second,
}

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
	url := fmt.Sprintf("http://%s:%d/", ip, port)
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

func GetMetrics(t *testing.T, ip string, port int) string {
	t.Helper()

	url := fmt.Sprintf("http://%s:%d/metrics", ip, port)

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
