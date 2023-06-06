// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package helpers

import (
	"fmt"
	"io"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// EnvoyAdminPort is the port Consul Dataplane will bind the Envoy admin server to.
var EnvoyAdminPort = TCP(30000)

type DataplaneConfig struct {
	Addresses                  string
	ServiceNodeName            string
	ProxyServiceID             string
	LoginAuthMethod            string
	LoginBearerToken           string
	DNSBindPort                string
	ServiceMetricsURL          string
	ShutdownGracePeriodSeconds string
	ShutdownDrainListeners     bool
}

func (cfg DataplaneConfig) ToArgs() []string {
	args := []string{
		"-addresses", cfg.Addresses,
		"-service-node-name", cfg.ServiceNodeName,
		"-proxy-service-id", cfg.ProxyServiceID,
		"-envoy-admin-bind-address", "0.0.0.0",
		"-envoy-admin-bind-port", EnvoyAdminPort.Port(),
		"-credential-type", "login",
		"-login-auth-method", cfg.LoginAuthMethod,
		"-login-bearer-token", cfg.LoginBearerToken,
		"-ca-certs", "/data/ca-cert.pem",
		"-tls-server-name", "server.dc1.consul",
		"-log-level", "debug",
		"-consul-dns-bind-port", cfg.DNSBindPort,
		"-telemetry-use-central-config",
		"-telemetry-prom-scrape-path", "/metrics",
		"-telemetry-prom-service-metrics-url", cfg.ServiceMetricsURL,
	}

	if cfg.ShutdownGracePeriodSeconds != "" {
		args = append(args, "-shutdown-grace-period-seconds", cfg.ShutdownGracePeriodSeconds)
	}

	if cfg.ShutdownDrainListeners {
		args = append(args, "-shutdown-drain-listeners")
	}

	return args
}

// RunDataplane runs consul-dataplane in the given pod's network. It captures
// the Envoy proxy's config as an artifact at the end of the test.
func RunDataplane(t *testing.T, pod *Pod, suite *Suite, cfg DataplaneConfig) *Container {
	t.Helper()

	volume := suite.Volume(t)

	container := suite.RunContainer(t, fmt.Sprintf("%s-dataplane", cfg.ProxyServiceID), true, ContainerRequest{
		NetworkMode: pod.Network(),
		Image:       suite.opts.DataplaneImage,
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
			pod.MappedPorts[EnvoyAdminPort],
		)

		// TODO: This will fail because the integration test now performs a
		// graceful shutdown of Envoy before reaching this point.
		rsp, err := httpClient.Get(url)
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
