package integrationtests

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type DataplaneConfig struct {
	Addresses        string
	ServiceNodeName  string
	ProxyServiceID   string
	LoginAuthMethod  string
	LoginBearerToken string
	DNSBindPort      string
}

func (cfg DataplaneConfig) ToArgs() []string {
	args := []string{
		"-addresses", cfg.Addresses,
		"-service-node-name", cfg.ServiceNodeName,
		"-proxy-service-id", cfg.ProxyServiceID,
		"-envoy-admin-bind-address", "0.0.0.0",
		"-envoy-admin-bind-port", envoyAdminPort.Port(),
		"-credential-type", "login",
		"-login-auth-method", cfg.LoginAuthMethod,
		"-login-bearer-token", cfg.LoginBearerToken,
		"-ca-certs", "/data/ca-cert.pem",
		"-tls-server-name", "server.dc1.consul",
		"-log-level", "debug",
		"-consul-dns-bind-port", cfg.DNSBindPort,
	}
	return args
}

// RunDataplane runs consul-dataplane in the given pod's network. It captures
// the Envoy proxy's config as an artifact at the end of the test.
func RunDataplane(t *testing.T, pod *Container, suite *Suite, cfg DataplaneConfig) *Container {
	t.Helper()

	volume := suite.Volume(t)

	container := suite.RunContainer(t, fmt.Sprintf("%s-dataplane", cfg.ProxyServiceID), true, ContainerRequest{
		NetworkMode: pod.Network(),
		Image:       dataplaneImage,
		Cmd:         cfg.ToArgs(),
		Mounts: []testcontainers.ContainerMount{
			testcontainers.VolumeMount(volume.Name, "/data"),
		},
		WaitingFor: wait.ForLog("starting main dispatch loop"), // https://github.com/envoyproxy/envoy/blob/ce49966ecb5f2d530117a29ae60b88198746fd74/source/server/server.cc#L906-L907
	})

	t.Cleanup(func() {
		url := fmt.Sprintf(
			"http://%s:%d/config_dump?include_eds",
			pod.HostIP,
			pod.MappedPorts[envoyAdminPort],
		)

		rsp, err := http.Get(url)
		if err != nil {
			t.Logf("failed to dump Envoy config: %v\n", err)
			return
		}
		defer rsp.Body.Close()

		config, err := io.ReadAll(rsp.Body)
		if err != nil {
			t.Logf("failed to dump Envoy config: %v\n", err)
			return
		}
		suite.CaptureArtifact(
			fmt.Sprintf("%s-envoy-config.json", cfg.ProxyServiceID),
			config,
		)
	})

	return container
}
