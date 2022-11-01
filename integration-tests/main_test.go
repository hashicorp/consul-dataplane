package integrationtests

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"

	. "github.com/hashicorp/consul-dataplane/integration-tests/helpers"
)

var (
	// upstreamLocalBindPort is the port the frontend sidecar will bind the local
	// listener for its backend upstream to.
	upstreamLocalBindPort = TCP(10000)

	// proxyInboundListenerPort is the port the sidecars will bind their public
	// listeners to. Only the backend sidecar's public port is used in these tests.
	proxyInboundListenerPort = TCP(20000)

	// dnsUDPPort is UDP the port Consul Dataplane's DNS proxy wil be bound to.
	dnsUDPPort = UDP(40000)

	// dnsTCPPort is TCP the port Consul Dataplane's DNS proxy wil be bound to.
	dnsTCPPort = TCP(40000)

	// metricsPort is the port Consul Dataplane will serve merged prometheus
	// metrics on.
	metricsPort = TCP(50000)

	// opts are the options used to configure the test suite (e.g. Consul server
	// image, output directory) set by flags in TestMain.
	opts SuiteOptions
)

func TestMain(m *testing.M) {
	flag.StringVar(&opts.ServerImage, "server-image", "hashicorppreview/consul:1.14-dev-39f665a1ef63ef31adee30f62244f9a9143464cd", "")
	flag.StringVar(&opts.DataplaneImage, "dataplane-image", "hashicorp/consul-dataplane:1.0.0-beta2", "")
	flag.StringVar(&opts.OutputDir, "output-dir", "", "")
	flag.BoolVar(&opts.DisableReaper, "disable-reaper", false, "")
	flag.Parse()

	if opts.OutputDir != "" {
		if err := os.MkdirAll(opts.OutputDir, 0770); err != nil {
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
//	* Creating proxy-defaults to set the default protocol to HTTP and prometheus
//	  bind address.
//	* Creating an L7/HTTP intention to allow "frontend" to talk to "backend".
//	* Making an HTTP request through the "frontend" sidecar's exposed "backend"
//	  port.
//	* Setting the intention action to deny.
//	* Attempting to make the same request and checking that it fails.
//	* Making DNS queries against the frontend dataplane's UDP and TCP DNS proxies.
//	* Scraping the prometheus merged metrics endpoint.
func TestIntegration(t *testing.T) {
	suite := NewSuite(t, opts)

	server := RunServer(t, suite)

	authMethod := NewAuthMethod(t)
	authMethod.Register(t, server)

	server.SetConfigEntry(t, &api.ProxyConfigEntry{
		Kind: api.ProxyDefaults,
		Name: api.ProxyConfigGlobal,
		Config: map[string]any{
			"protocol":                   "http",
			"envoy_prometheus_bind_addr": net.JoinHostPort("0.0.0.0", metricsPort.Port()),
		},
	})

	server.RegisterSyntheticNode(t)

	server.RegisterService(t, &api.AgentService{
		Service: "backend",
		Port:    8080,
	})

	backendPod := RunPod(t, suite, "backend", []nat.Port{
		EnvoyAdminPort,
		metricsPort,
	})

	server.RegisterService(t, &api.AgentService{
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
		Addresses:         server.Container.ContainerIP,
		ServiceNodeName:   SyntheticNodeName,
		ProxyServiceID:    "backend-sidecar",
		LoginAuthMethod:   authMethod.Name,
		LoginBearerToken:  authMethod.GenerateToken(t, "backend"),
		DNSBindPort:       dnsUDPPort.Port(),
		ServiceMetricsURL: "http://localhost:8080",
	})

	frontendPod := RunPod(t, suite, "frontend", []nat.Port{
		EnvoyAdminPort,
		upstreamLocalBindPort,
		dnsUDPPort,
		dnsTCPPort,
	})

	server.RegisterService(t, &api.AgentService{
		Service: "frontend",
		Port:    8080,
	})

	server.RegisterService(t, &api.AgentService{
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
		Addresses:         server.Container.ContainerIP,
		ServiceNodeName:   SyntheticNodeName,
		ProxyServiceID:    "frontend-sidecar",
		LoginAuthMethod:   authMethod.Name,
		LoginBearerToken:  authMethod.GenerateToken(t, "frontend"),
		DNSBindPort:       dnsUDPPort.Port(),
		ServiceMetricsURL: "http://localhost:8080",
	})

	server.SetConfigEntry(t, &api.ServiceIntentionsConfigEntry{
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

	server.SetConfigEntry(t, &api.ServiceIntentionsConfigEntry{
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

	dnsPorts := []nat.Port{dnsUDPPort, dnsTCPPort}
	frontendPod.ExposeInternalPorts(t, dnsPorts)

	for _, port := range dnsPorts {
		addrs := DNSLookup(t,
			suite,
			port.Proto(),
			frontendPod.HostIP,
			frontendPod.MappedPorts[port],
			"backend-sidecar.service.consul.",
		)
		require.ElementsMatch(t, []string{backendPod.ContainerIP}, addrs)
	}

	metrics := GetMetrics(t,
		backendPod.HostIP,
		backendPod.MappedPorts[metricsPort],
	)
	require.Contains(t, metrics, "consul_dataplane_go_goroutines")
	require.Contains(t, metrics, "envoy_server_total_connections")
	require.Contains(t, metrics, `service_metric{service_name="backend"}`)
}
