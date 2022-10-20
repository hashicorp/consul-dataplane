package integrationtests

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/docker/go-connections/nat"

	"github.com/hashicorp/consul/api"
)

// syntheticNodeName is the name given to the "synthetic" node services are
// registered to.
const syntheticNodeName = "synthetic-node"

var (
	// serverImage is the container image reference for the Consul server. It can
	// be configured using the -server-image flag.
	serverImage string

	// dataplaneImage is the container image reference for Consul Dataplane. It
	// can be configured using the -dataplane-image flag.
	dataplaneImage string

	// outputDir is the directory artifacts will be written to. It can be configured
	// using the -output-dir flag.
	outputDir string

	// disableReaper controls whether the container reaper is enabled. It can be
	// configured using the -disable-reaper flag.
	//
	// See: https://hub.docker.com/r/testcontainers/ryuk
	disableReaper bool

	// upstreamLocalBindPort is the port the frontend sidecar will bind the local
	// listener for its backend upstream to.
	upstreamLocalBindPort = tcpPort(10000)

	// proxyInboundListenerPort is the port the sidecars will bind their public
	// listeners to. Only the backend sidecar's public port is used in these tests.
	proxyInboundListenerPort = tcpPort(20000)

	// envoyAdminPort is the port both sidecars will bind the Envoy admin server
	// to.
	envoyAdminPort = tcpPort(30000)

	// serverHTTPPort is the port the Consul server's HTTP interface will be bound
	// to.
	serverHTTPPort = tcpPort(8500)
)

func TestMain(m *testing.M) {
	flag.StringVar(&serverImage, "server-image", "hashicorppreview/consul:1.14-dev-39f665a1ef63ef31adee30f62244f9a9143464cd", "")
	flag.StringVar(&dataplaneImage, "dataplane-image", "hashicorp/consul-dataplane:1.0.0-beta2", "")
	flag.StringVar(&outputDir, "output-dir", "", "")
	flag.BoolVar(&disableReaper, "disable-reaper", false, "")
	flag.Parse()

	if outputDir != "" {
		if err := os.MkdirAll(outputDir, 0770); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create -output-dir: %v", err)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

// TestIntegration covers the end-to-end service mesh flow by:
//
//	* Running a Consul server with TLS and ACLs enabled.
//	* Creating a JWT ACL auth-method.
//	* Registering two services and sidecars ("frontend" and "backend") with an
//	  upstream relationship.
//	* Running a simple HTTP server for the "backend" service.
//	* Running consul-datplane for each sidecar, with the "frontend" sidecar's
//	  local listener port for its "backend" upstream exposed to the host.
//	* Creating proxy-defaults to set the default protocol to HTTP.
//	* Creating an L7/HTTP intention to allow "frontend" to talk to "backend".
//	* Making an HTTP request through the "frontend" sidecar's exposed "backend"
//	  port.
//	* Setting the intention action to deny.
//	* Attempting to make the same request and checking that it fails.
func TestIntegration(t *testing.T) {
	suite := NewSuite(t)

	server := RunServer(t, suite)
	client := server.Client(t)

	authMethod := NewAuthMethod(t)
	authMethod.Register(t, client)

	RegisterSytheticNode(t, client)

	RegisterService(t, client, &api.AgentService{
		Service: "backend",
		Port:    8080,
	})

	backendPod := RunPod(t, suite, "backend", []nat.Port{
		envoyAdminPort,
	})

	RegisterService(t, client, &api.AgentService{
		Service: "backend-sidecar",
		Kind:    api.ServiceKindConnectProxy,
		Port:    proxyInboundListenerPort.Int(),
		Address: backendPod.ContainerIP,
		Proxy: &api.AgentServiceConnectProxyConfig{
			DestinationServiceName: "backend",
			LocalServicePort:       8080,
		},
	})

	RunService(t, suite, backendPod, "backend")

	RunDataplane(t, backendPod, suite, DataplaneConfig{
		Addresses:        server.Container.ContainerIP,
		ServiceNodeName:  syntheticNodeName,
		ProxyServiceID:   "backend-sidecar",
		LoginAuthMethod:  authMethod.name(),
		LoginBearerToken: authMethod.GenerateToken(t, "backend"),
	})

	frontendPod := RunPod(t, suite, "frontend", []nat.Port{
		envoyAdminPort,
		upstreamLocalBindPort,
	})

	RegisterService(t, client, &api.AgentService{
		Service: "frontend",
		Port:    8080,
	})

	RegisterService(t, client, &api.AgentService{
		Service: "frontend-sidecar",
		Kind:    api.ServiceKindConnectProxy,
		Port:    proxyInboundListenerPort.Int(),
		Address: frontendPod.ContainerIP,
		Proxy: &api.AgentServiceConnectProxyConfig{
			DestinationServiceName: "frontend",
			Upstreams: []api.Upstream{
				{
					DestinationType:  api.UpstreamDestTypeService,
					DestinationName:  "backend",
					LocalBindPort:    upstreamLocalBindPort.Int(),
					LocalBindAddress: "0.0.0.0",
				},
			},
		},
	})

	RunDataplane(t, frontendPod, suite, DataplaneConfig{
		Addresses:        server.Container.ContainerIP,
		ServiceNodeName:  syntheticNodeName,
		ProxyServiceID:   "frontend-sidecar",
		LoginAuthMethod:  authMethod.name(),
		LoginBearerToken: authMethod.GenerateToken(t, "frontend"),
	})

	SetConfigEntry(t, client, &api.ProxyConfigEntry{
		Kind: api.ProxyDefaults,
		Name: api.ProxyConfigGlobal,
		Config: map[string]any{
			"protocol": "http",
		},
	})

	SetConfigEntry(t, client, &api.ServiceIntentionsConfigEntry{
		Kind: api.ServiceIntentions,
		Name: "backend",
		Sources: []*api.SourceIntention{
			{
				Name: "frontend",
				Type: api.IntentionSourceConsul,
				Permissions: []*api.IntentionPermission{
					{
						Action: api.IntentionActionAllow,
						HTTP: &api.IntentionHTTPPermission{
							PathPrefix: "/",
							Methods:    []string{http.MethodGet},
						},
					},
				},
			},
		},
	})

	ExpectHTTPAccess(t,
		frontendPod.HostIP,
		frontendPod.MappedPorts[upstreamLocalBindPort],
	)

	SetConfigEntry(t, client, &api.ServiceIntentionsConfigEntry{
		Kind: api.ServiceIntentions,
		Name: "backend",
		Sources: []*api.SourceIntention{
			{
				Name:   "frontend",
				Action: api.IntentionActionDeny,
				Type:   api.IntentionSourceConsul,
			},
		},
	})

	ExpectNoHTTPAccess(t,
		frontendPod.HostIP,
		frontendPod.MappedPorts[upstreamLocalBindPort],
	)
}
