// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package integrationtests

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"

	. "github.com/hashicorp/consul-dataplane/integration-tests/helpers"
)

var (
	// upstreamLocalBindPort is the port each sidecar will bind the local
	// listener for its upstream to.
	upstreamLocalBindPort = TCP(10000)

	// proxyInboundListenerPort is the port the sidecars will bind their public
	// listeners to.
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
	flag.StringVar(&opts.ServerImage, "server-image", "hashicorppreview/consul:1.15-dev", "")
	flag.StringVar(&opts.ServerVersion, "server-version", "v1.15.0-dev", "")
	flag.StringVar(&opts.DataplaneImage, "dataplane-image", "consul-dataplane/release-default:1.0.0-dev", "")
	flag.StringVar(&opts.OutputDir, "output-dir", "", "")
	flag.BoolVar(&opts.DisableReaper, "disable-reaper", false, "")
	flag.Parse()

	if !semver.IsValid(semver.MajorMinor(opts.ServerVersion)) {
		fmt.Fprintf(os.Stderr, "invalid semver %s for -server-version", opts.ServerVersion)
		os.Exit(1)
	}

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
//   - Running a Consul server with TLS and ACLs enabled.
//   - Creating a JWT ACL auth-method.
//   - Registering two services and sidecars ("frontend" and "backend") with an
//     upstream relationship.
//   - Running a simple HTTP server for the "backend" service.
//   - Running consul-datplane for each sidecar, with the "frontend" sidecar's
//     local listener port for its "backend" upstream exposed to the host.
//   - Creating proxy-defaults to set the default protocol to HTTP and prometheus
//     bind address. Also set access logs on the admin interface of Envoy
//   - Creating an L7/HTTP intention to allow "frontend" to talk to "backend".
//   - Making an HTTP request through the "frontend" sidecar's exposed "backend"
//     port.
//   - Setting the intention action to deny.
//   - Attempting to make the same request and checking that it fails.
//   - Making DNS queries against the frontend dataplane's UDP and TCP DNS proxies.
//   - Scraping the prometheus merged metrics endpoint.
//   - Make a call to Envoy's admin interface and check for the access logs.
func TestIntegration(t *testing.T) {
	suite := NewSuite(t, opts)

	server := RunServer(t, suite)

	authMethod := NewAuthMethod(t)
	authMethod.Register(t, server)

	proxyDefault := &api.ProxyConfigEntry{
		Kind: api.ProxyDefaults,
		Name: api.ProxyConfigGlobal,
		Config: map[string]any{
			"protocol":                   "http",
			"envoy_prometheus_bind_addr": net.JoinHostPort("0.0.0.0", metricsPort.Port()),
		},
	}

	// Consul 1.15 supports access logs
	if semver.Compare(semver.MajorMinor(opts.ServerVersion), "v1.15") >= 0 {
		proxyDefault.AccessLogs = &api.AccessLogsConfig{
			Enabled:    true,
			JSONFormat: "{\"custom_field_path\":\"%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%\"}",
		}
	}

	server.SetConfigEntry(t, proxyDefault)

	server.RegisterSyntheticNode(t)

	backendPod := RunPod(t, suite, "backend", []nat.Port{
		EnvoyAdminPort,
		upstreamLocalBindPort,
		metricsPort,
	})

	server.RegisterService(t, &api.AgentService{
		Service: "backend",
		Port:    8080,
	})

	server.RegisterService(t, &api.AgentService{
		Service: "backend-sidecar",
		Kind:    api.ServiceKindConnectProxy,
		Port:    proxyInboundListenerPort.Int(),
		Address: backendPod.ContainerIP,
		Proxy: &api.AgentServiceConnectProxyConfig{
			DestinationServiceName: "backend",
			LocalServicePort:       8080,
			Upstreams: []api.Upstream{
				{
					DestinationType:  api.UpstreamDestTypeService,
					DestinationName:  "frontend",
					LocalBindPort:    upstreamLocalBindPort.Int(),
					LocalBindAddress: "0.0.0.0",
				},
			},
		},
	})

	RunService(t, suite, backendPod, "backend")

	backendDataplane := RunDataplane(t, backendPod, suite, DataplaneConfig{
		Addresses:                    server.Container.ContainerIP,
		ServiceNodeName:              SyntheticNodeName,
		ProxyServiceID:               "backend-sidecar",
		LoginAuthMethod:              authMethod.Name,
		LoginBearerToken:             authMethod.GenerateToken(t, "backend"),
		DNSBindPort:                  dnsUDPPort.Port(),
		ServiceMetricsURL:            "http://localhost:8080",
		DumpEnvoyConfigOnExitEnabled: true,
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
			LocalServicePort:       8080,
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

	RunService(t, suite, frontendPod, "frontend")

	frontendDataplane := RunDataplane(t, frontendPod, suite, DataplaneConfig{
		Addresses:                     server.Container.ContainerIP,
		ServiceNodeName:               SyntheticNodeName,
		ProxyServiceID:                "frontend-sidecar",
		LoginAuthMethod:               authMethod.Name,
		LoginBearerToken:              authMethod.GenerateToken(t, "frontend"),
		DNSBindPort:                   dnsUDPPort.Port(),
		ServiceMetricsURL:             "http://localhost:8080",
		ShutdownGracePeriodSeconds:    "10",
		ShutdownDrainListenersEnabled: true,
		DumpEnvoyConfigOnExitEnabled:  true,
	})

	// Intentions are configured as default deny in helpers/server.go
	ExpectNoHTTPAccess(t,
		frontendPod.HostIP,
		frontendPod.MappedPorts[upstreamLocalBindPort],
	)

	ExpectNoHTTPAccess(t,
		backendPod.HostIP,
		backendPod.MappedPorts[upstreamLocalBindPort],
	)

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

	adminIP := frontendPod.HostIP
	adminPort := frontendPod.MappedPorts[EnvoyAdminPort]

	// Structural check: the inline (virtual) and egress DNS listeners are
	// provisioned onto Envoy by the Consul server over xDS. This single check
	// confirms whether the server pushed them to this proxy, independent of any
	// DNS query. Their names are "virtual_dns:127.0.0.1:8653" and
	// "egress_dns:127.0.0.1:8654" (see consul-enterprise/agent/xds/listeners_dns.go).
	//
	//   - The inline virtual DNS listener is present whenever the running Consul
	//     server supports it.
	//   - The egress listener is only pushed when recursors are configured.
	//
	// Presence is reported for visibility but not asserted, since it depends on
	// the Consul server version and recursor configuration.
	hasInlineListener := EnvoyHasListener(t, adminIP, adminPort, "virtual_dns")
	hasEgressListener := EnvoyHasListener(t, adminIP, adminPort, "egress_dns")
	t.Logf("Envoy consul DNS listeners: consul_server_version=%s inline_virtual_dns=%t egress_dns=%t",
		opts.ServerVersion, hasInlineListener, hasEgressListener)

	// The remaining subtests only verify that DNS resolution works for each kind
	// of domain, over both UDP and TCP. They assert on observable behavior
	// (resolved addresses / rcode) and do not care which internal path served the
	// query.

	// Standard .consul service discovery must resolve to the backend pod IP.
	t.Run("service .consul lookup", func(t *testing.T) {
		for _, port := range dnsPorts {
			addrs := DNSLookup(t,
				suite,
				port.Proto(),
				frontendPod.HostIP,
				frontendPod.MappedPorts[port],
				"backend-sidecar.service.consul.",
			)

			t.Logf("DNS service lookup: consul_server_version=%s proto=%s query=%s addrs=%v",
				opts.ServerVersion, port.Proto(), "backend-sidecar.service.consul.", addrs)

			require.ElementsMatch(t, []string{backendPod.ContainerIP}, addrs)
		}
	})

	// A *.virtual.consul query for the "backend" upstream must resolve to a
	// Consul VIP from the 240.0.0.0/4 range.
	t.Run("virtual .virtual.consul lookup", func(t *testing.T) {
		for _, port := range dnsPorts {
			addrs, rcode := DNSLookupA(t,
				suite,
				port.Proto(),
				frontendPod.HostIP,
				frontendPod.MappedPorts[port],
				"backend.virtual.consul.",
			)

			t.Logf("DNS virtual lookup: consul_server_version=%s proto=%s query=%s rcode=%d addrs=%v",
				opts.ServerVersion, port.Proto(), "backend.virtual.consul.", rcode, addrs)

			require.Equalf(t, DNSRcodeSuccess, rcode,
				"expected backend.virtual.consul to resolve successfully over %s, got rcode=%d",
				port.Proto(), rcode)
			require.NotEmptyf(t, addrs,
				"expected backend.virtual.consul to resolve to a VIP over %s, got no answers",
				port.Proto())

			for _, addr := range addrs {
				require.Truef(t, IsConsulVIP(addr),
					"expected backend.virtual.consul to resolve to a Consul VIP (240.0.0.0/4), got %q over %s",
					addr, port.Proto())
			}
		}
	})

	// A non-.consul domain must resolve via the configured recursors (8.8.8.8 /
	// 8.8.4.4, set on the Consul server in helpers/server.go). Whether the query
	// is served by Envoy's egress DNS listener or by the Consul-server recursor
	// fallback, "google.com" must resolve successfully to at least one address.
	t.Run("external domain lookup", func(t *testing.T) {
		for _, port := range dnsPorts {
			addrs, rcode := DNSLookupA(t,
				suite,
				port.Proto(),
				frontendPod.HostIP,
				frontendPod.MappedPorts[port],
				"google.com.",
			)

			t.Logf("DNS external lookup: consul_server_version=%s proto=%s query=%s rcode=%d addrs=%v",
				opts.ServerVersion, port.Proto(), "google.com.", rcode, addrs)

			require.Equalf(t, DNSRcodeSuccess, rcode,
				"expected google.com to resolve successfully via recursors over %s, got rcode=%d",
				port.Proto(), rcode)
			require.NotEmptyf(t, addrs,
				"expected google.com to resolve to at least one address via recursors over %s, got no answers",
				port.Proto())
		}
	})

	metrics := GetMetrics(t,
		backendPod.HostIP,
		backendPod.MappedPorts[metricsPort],
	)
	require.Contains(t, metrics, "consul_dataplane_go_goroutines")
	require.Contains(t, metrics, "envoy_server_total_connections")
	require.Contains(t, metrics, `service_metric{service_name="backend"}`)

	// Test access logs (Consul 1.15 or greater)
	if semver.Compare(semver.MajorMinor(opts.ServerVersion), "v1.15") >= 0 {
		GetEnvoyClusters(t, backendPod.HostIP, backendPod.MappedPorts[EnvoyAdminPort])
		require.Eventuallyf(t, func() bool {
			output := backendDataplane.ContainerLogs(t)
			return strings.Contains(output, "{\"custom_field_path\":\"/clusters\"}")
		}, 30*time.Second, 3*time.Second, "could not find admin access logs in output")
	}

	// Overwrite deny intention and allow two-way connections to prepare for
	// testing graceful shutdown
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
	server.SetConfigEntry(t, &api.ServiceIntentionsConfigEntry{
		Kind: api.ServiceIntentions,
		Name: "frontend",
		Sources: []*api.SourceIntention{
			{
				Name: "backend",
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

	// Ensure frontend upstream on backend service is working
	ExpectHTTPAccess(t,
		backendPod.HostIP,
		backendPod.MappedPorts[upstreamLocalBindPort],
	)

	// virtual .virtual.consul resolves after consul server outage
	//
	// Consul servers >= v2.2 push Envoy DNS listeners (the inline virtual-DNS
	// table) over xDS, so Envoy can continue answering *.virtual.consul queries
	// from its local table after the control plane is gone. On older servers the
	// listener is never pushed, so resolution after an outage is not expected to
	// work. The test is therefore only asserted on v2.2+; on earlier versions it
	// is skipped to avoid a spurious failure.
	t.Run("virtual .virtual.consul resolves after consul server outage", func(t *testing.T) {
		// Determine whether the running Consul server is new enough to push the
		// inline virtual-DNS listener over xDS. The feature landed in v2.2.
		supportsInlineDNS := semver.Compare(semver.MajorMinor(opts.ServerVersion), "v2.2") >= 0

		serverContainerID := server.Container.GetContainerID()
		dockerCli, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation())
		if err != nil {
			t.Fatalf("failed to create docker client: %v", err)
		}
		if err := dockerCli.ContainerKill(context.Background(), serverContainerID, "SIGKILL"); err != nil {
			t.Fatalf("failed to kill consul server container %s: %v", serverContainerID, err)
		}
		t.Logf("consul server container %s killed (SIGKILL) to simulate control-plane outage", serverContainerID)

		if !supportsInlineDNS {
			t.Skipf("skipping outage-resilience check: consul server %s < v2.2 does not push inline virtual-DNS listeners over xDS",
				opts.ServerVersion)
		}

		// Query with retries: after a hard server kill the Envoy DNS proxy is
		// still alive and serving the inline table, but the very first UDP
		// exchange may time out while the proxy's upstream health-check to
		// Consul drains. Retry for up to 15 s before failing.
		require.Eventually(t, func() bool {
			for _, port := range dnsPorts {
				addrs, rcode, err := DNSLookupAErr(suite,
					port.Proto(),
					frontendPod.HostIP,
					frontendPod.MappedPorts[port],
					"backend.virtual.consul.",
				)
				if err != nil {
					t.Logf("DNS virtual lookup transient error (retrying): consul_server_version=%s proto=%s err=%v",
						opts.ServerVersion, port.Proto(), err)
					return false
				}

				t.Logf("DNS virtual lookup: consul_server_version=%s proto=%s query=%s rcode=%d addrs=%v",
					opts.ServerVersion, port.Proto(), "backend.virtual.consul.", rcode, addrs)

				if rcode != DNSRcodeSuccess || len(addrs) == 0 {
					return false
				}
				for _, addr := range addrs {
					if !IsConsulVIP(addr) {
						return false
					}
				}
			}
			return true
		}, 15*time.Second, 1*time.Second,
			"expected backend.virtual.consul to resolve to a Consul VIP via Envoy inline DNS table after control-plane outage")
	})

	// Send SIGTERM to dataplane to start graceful shutdown
	containerID := frontendDataplane.Container.GetContainerID()
	cli, err := client.NewClientWithOpts(
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error initializing docker client: %s\n", err)
		os.Exit(1)
	}
	err = cli.ContainerKill(context.Background(), containerID, "SIGTERM")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error killing docker container %s: %s\n", containerID, err)
		os.Exit(1)
	}
	// TODO: It may be preferrable to use ContainerStop to set a longer
	// StopTimeout to avoid issues with cleanup, but importing the
	// docker/docker/container package for StopOptions has dependency issues.
	// https://pkg.go.dev/github.com/docker/docker/client#Client.ContainerStop
	// err = cli.ContainerStop(context.Background(), containerID, container.StopOptions{})

	// Expect outgoing connections through sidecar are allowed until shutdown
	// grace period has elapsed.
	ExpectHTTPAccess(t,
		frontendPod.HostIP,
		frontendPod.MappedPorts[upstreamLocalBindPort],
	)

	// Expect inbound connections to the frontend service are rejected while it
	// is shutting down if listener draining is configured.
	ExpectNoHTTPAccess(t,
		backendPod.HostIP,
		backendPod.MappedPorts[upstreamLocalBindPort],
	)
}
