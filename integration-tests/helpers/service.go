package helpers

import (
	"fmt"
	"testing"
)

const echoServiceImage = "ttl.sh/dans/http-echo:latest"

// RunService runs an HTTP echo server in the given pod's network, running on
// port :8080.
func RunService(t *testing.T, suite *Suite, pod *Pod, serviceName string) *Container {
	t.Helper()

	return suite.RunContainer(t, serviceName, true, ContainerRequest{
		NetworkMode: pod.Network(),
		Image:       echoServiceImage,
		Cmd:         []string{"-listen", ":8080", "-text", fmt.Sprintf("service_metric{service_name=%q} 1", serviceName)},
	})
}
