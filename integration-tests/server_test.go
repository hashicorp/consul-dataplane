package integrationtests

import (
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/hashicorp/consul/api"
)

const rootACLToken = "1e7038d4-53ff-4c18-a0c0-1e72d9c101dc"

var serverConfig = `
server = true
data_dir = "/consul/data"
log_level = "debug"

bootstrap_expect = 1

acl {
	enabled = true
	default_policy = "deny"

	tokens {
		initial_management = "` + rootACLToken + `"
		default = "` + rootACLToken + `"
	}
}

connect {
	enabled = true
}

ports {
	http = ` + serverHTTPPort.Port() + `
	grpc_tls = 8502
}

tls {
	grpc {
		ca_file = "/data/ca-cert.pem"
		cert_file = "/data/server-cert.pem"
		key_file = "/data/server-key.pem"
	}
}
`

type ConsulServer struct {
	Container *Container
}

// Client returns a Consul API client configured to talk to the server.
func (c ConsulServer) Client(t *testing.T) *api.Client {
	t.Helper()

	client, err := api.NewClient(&api.Config{
		Address: net.JoinHostPort(c.Container.HostIP, strconv.Itoa(c.Container.MappedPorts[serverHTTPPort])),
		Token:   rootACLToken,
	})
	require.NoError(t, err)

	return client
}

// RunServer runs a Consul server.
func RunServer(t *testing.T, suite *Suite) ConsulServer {
	t.Helper()

	GenerateServerTLS(t, suite)

	volume := suite.Volume(t)
	volume.WriteFile(t, "server.hcl", []byte(serverConfig))

	container := suite.RunContainer(t, "server", true, ContainerRequest{
		Image: serverImage,
		Mounts: []testcontainers.ContainerMount{
			testcontainers.VolumeMount(volume.Name, "/data"),
		},
		ExposedPorts: []string{"8500/tcp"},
		WaitingFor:   wait.ForLog("New leader elected"),
		Cmd:          []string{"consul", "agent", "-config-file", "/data/server.hcl", "-client", "0.0.0.0"},
	})

	return ConsulServer{Container: container}
}
