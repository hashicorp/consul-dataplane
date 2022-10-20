package integrationtests

import (
	"fmt"
	"testing"

	"github.com/docker/go-connections/nat"
)

const pauseContainerImage = "google/pause:asm"

// RunPod runs a "pause" container with the given ports mapped to the host, the
// network namespace of which will be joined by other containers. This allows
// us to know the IP address before starting the "real" container (as is needed
// to register a sidecar proxy before running consul-dataplane).
func RunPod(t *testing.T, suite *Suite, podName string, mappedPorts []nat.Port) *Container {
	t.Helper()

	exposedPorts := make([]string, len(mappedPorts))
	for idx, port := range mappedPorts {
		exposedPorts[idx] = string(port)
	}

	return suite.RunContainer(t, fmt.Sprintf("%s-pod", podName), false, ContainerRequest{
		Image:        pauseContainerImage,
		ExposedPorts: exposedPorts,
	})
}
