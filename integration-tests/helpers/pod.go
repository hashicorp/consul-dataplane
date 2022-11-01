package helpers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
)

const pauseContainerImage = "google/pause:asm"

type Pod struct {
	*Container

	suite *Suite
}

// RunPod runs a "pause" container with the given ports mapped to the host, the
// network namespace of which will be joined by other containers. This allows
// us to know the IP address before starting the "real" container (as is needed
// to register a sidecar proxy before running consul-dataplane).
func RunPod(t *testing.T, suite *Suite, podName string, mappedPorts []nat.Port) *Pod {
	t.Helper()

	exposedPorts := make([]string, len(mappedPorts))
	for idx, port := range mappedPorts {
		exposedPorts[idx] = string(port)
	}

	container := suite.RunContainer(t, fmt.Sprintf("%s-pod", podName), false, ContainerRequest{
		Image:        pauseContainerImage,
		ExposedPorts: exposedPorts,
	})

	return &Pod{
		Container: container,
		suite:     suite,
	}
}

// ExposeInternalPorts creates iptables rules to forward traffic from
// public-facing ports to those bound only on the loopback interface. This
// is useful for testing our DNS proxy which can **only** be bound to the
// loopback interace.
func (p *Pod) ExposeInternalPorts(t *testing.T, ports []nat.Port) {
	t.Helper()

	container := p.suite.RunContainer(t, "expose-internal-pods", false, ContainerRequest{
		NetworkMode: p.Network(),
		Image:       "alpine:3.16.2",
		Cmd:         []string{"sleep", "infinity"},
		Privileged:  true,
	})

	cmds := []string{
		"sysctl -w net.ipv4.conf.all.route_localnet=1",
		"apk add iptables",
	}

	for _, port := range ports {
		cmds = append(cmds, fmt.Sprintf(
			"iptables -t nat -A PREROUTING -p %s --dport %s -j DNAT --to-destination 127.0.0.1:%s",
			port.Proto(),
			port.Port(),
			port.Port(),
		))
	}

	for _, cmd := range cmds {
		_, _, err := container.Exec(p.suite.Context(t), strings.Split(cmd, " "))
		require.NoError(t, err)
	}
}
