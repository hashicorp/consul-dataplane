package integrationtests

import (
	"fmt"
	"strings"
	"testing"

	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
)

// ExposeInternalPort creates iptables rules to forward traffic from a
// public-facing port to one bound only on the loopback interface. This
// is useful for testing our DNS proxy which can **only** be bound to the
// loopback interace.
func ExposeInternalPort(t *testing.T, suite *Suite, pod *Container, port nat.Port) {
	t.Helper()

	name := fmt.Sprintf("expose-%s-%s", port.Proto(), port.Port())
	container := suite.RunContainer(t, name, false, ContainerRequest{
		NetworkMode: pod.Network(),
		Image:       "alpine:3.16.2",
		Cmd:         []string{"sleep", "infinity"},
		Privileged:  true,
	})

	cmds := []string{
		"sysctl -w net.ipv4.conf.all.route_localnet=1",
		"apk add iptables",
		"iptables -t nat -A PREROUTING -p " + port.Proto() + " --dport " + port.Port() + " -j DNAT --to-destination 127.0.0.1:" + port.Port(),
	}

	for _, cmd := range cmds {
		_, _, err := container.Exec(suite.Context(t), strings.Split(cmd, " "))
		require.NoError(t, err)
	}
}
