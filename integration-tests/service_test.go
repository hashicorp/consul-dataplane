package integrationtests

import (
	"fmt"
	"testing"
)

const echoServiceImage = "hashicorp/http-echo:0.2.3"

// RunService runs an HTTP echo server in the given pod's network, running on
// port :8080.
func RunService(t *testing.T, suite *Suite, pod *Container, serviceName string) *Container {
	t.Helper()

	return suite.RunContainer(t, serviceName, true, ContainerRequest{
		NetworkMode: pod.Network(),
		Image:       echoServiceImage,
		Cmd:         []string{"-listen", ":8080", "-text", fmt.Sprintf("Response from %s", serviceName)},
	})
}
