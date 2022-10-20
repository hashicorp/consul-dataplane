package integrationtests

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
)

func tcpPort(n int) nat.Port {
	port, err := nat.NewPort("tcp", strconv.Itoa(n))
	if err != nil {
		panic(err)
	}
	return port
}

func RegisterSytheticNode(t *testing.T, client *api.Client) {
	t.Helper()

	_, err := client.Catalog().Register(&api.CatalogRegistration{
		Node:    syntheticNodeName,
		Address: "127.0.0.1",
	}, nil)
	require.NoError(t, err)
}

func RegisterService(t *testing.T, client *api.Client, service *api.AgentService) {
	t.Helper()

	_, err := client.Catalog().Register(&api.CatalogRegistration{
		Node:           syntheticNodeName,
		SkipNodeUpdate: true,
		Service:        service,
	}, nil)
	require.NoError(t, err)
}

func SetConfigEntry(t *testing.T, client *api.Client, entry api.ConfigEntry) {
	t.Helper()

	_, _, err := client.ConfigEntries().Set(entry, nil)
	require.NoError(t, err)
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
	rsp, err := http.Get(url)
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
