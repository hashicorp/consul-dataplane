// Copyright IBM Corp. 2022, 2025
// SPDX-License-Identifier: MPL-2.0

package helpers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

// GenerateServerTLS generates CA and server certificates/key material and
// copies them to the suite volume to be used by the server and dataplanes.
func GenerateServerTLS(t *testing.T, suite *Suite) {
	t.Helper()

	ctx := suite.Context(t)
	volume := suite.Volume(t)

	container := suite.RunContainer(t, "generate-tls-certs", false, ContainerRequest{
		Image: suite.opts.ServerImage,
		Cmd:   []string{"sleep", "infinity"},
		Mounts: []testcontainers.ContainerMount{
			testcontainers.VolumeMount(volume.Name, "/data"),
		},
	})

	cmds := []string{
		"consul tls ca create",
		"cp consul-agent-ca.pem /data/ca-cert.pem",
		"consul tls cert create -server",
		"cp dc1-server-consul-0.pem /data/server-cert.pem",
		"cp dc1-server-consul-0-key.pem /data/server-key.pem",
		"chmod 444 /data/server-key.pem",
	}
	for _, cmd := range cmds {
		_, _, err := container.Exec(ctx, strings.Split(cmd, " "))
		require.NoError(t, err)
	}
}
