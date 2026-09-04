// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package dns

import "testing"

func TestParseServiceSNI(t *testing.T) {
	const td = "e5b1a4d3.consul"
	cases := []struct {
		name string
		sni  string
		want UpstreamComponents
		ok   bool
	}{
		// ── baseline: single-port, default partition ──────────────────────────
		{
			name: "default partition no subset",
			sni:  "service-response.default.dc1.internal." + td,
			want: UpstreamComponents{Service: "service-response", Namespace: "default", Partition: "default", Datacenter: "dc1"},
			ok:   true,
		},
		{
			name: "default partition with subset",
			sni:  "v2.service-response.default.dc1.internal." + td,
			want: UpstreamComponents{Service: "service-response", Namespace: "default", Partition: "default", Datacenter: "dc1"},
			ok:   true,
		},

		// ── baseline: single-port, non-default partition ──────────────────────
		{
			name: "non-default partition no subset",
			sni:  "service-response.default.partition-vms.dc1.internal-v1." + td,
			want: UpstreamComponents{Service: "service-response", Namespace: "default", Partition: "partition-vms", Datacenter: "dc1"},
			ok:   true,
		},
		{
			name: "non-default partition with subset",
			sni:  "v2.service-response.ns1.partition-vms.dc2.internal-v1." + td,
			want: UpstreamComponents{Service: "service-response", Namespace: "ns1", Partition: "partition-vms", Datacenter: "dc2"},
			ok:   true,
		},

		// ── multi-port: default partition ────────────────────────────────────
		// ClusterNameWithPort("api-port", sni) prepends the port name as an
		// extra leading label. The port name must be stripped just like a
		// subset so the service identity is extracted correctly.
		{
			name: "default partition multi-port no subset",
			sni:  "api-port.api.default.dc1.internal." + td,
			want: UpstreamComponents{Service: "api", Namespace: "default", Partition: "default", Datacenter: "dc1"},
			ok:   true,
		},
		// When both a port name and a subset are present the cluster name has
		// two extra leading labels: <port>.<subset>.<svc>…
		{
			name: "default partition multi-port with subset",
			sni:  "api-port.v2.api.default.dc1.internal." + td,
			want: UpstreamComponents{Service: "api", Namespace: "default", Partition: "default", Datacenter: "dc1"},
			ok:   true,
		},

		// ── multi-port: non-default partition ────────────────────────────────
		{
			name: "non-default partition multi-port no subset",
			sni:  "grpc-port.svc.ns1.partition-vms.dc1.internal-v1." + td,
			want: UpstreamComponents{Service: "svc", Namespace: "ns1", Partition: "partition-vms", Datacenter: "dc1"},
			ok:   true,
		},
		{
			name: "non-default partition multi-port with subset",
			sni:  "grpc-port.v2.svc.ns1.partition-vms.dc1.internal-v1." + td,
			want: UpstreamComponents{Service: "svc", Namespace: "ns1", Partition: "partition-vms", Datacenter: "dc1"},
			ok:   true,
		},

		// ── passthrough~ prefix (transparent-proxy clusters) ─────────────────
		// The control plane names passthrough clusters as "passthrough~<sni>"
		// (agent/xds/clusters.go line 427). The "~" separator keeps the prefix
		// out of the dot-split label array, so stripping it before splitting
		// leaves a valid SNI to parse.
		{
			name: "passthrough prefix default partition no subset",
			sni:  "passthrough~api.default.dc1.internal." + td,
			want: UpstreamComponents{Service: "api", Namespace: "default", Partition: "default", Datacenter: "dc1"},
			ok:   true,
		},
		{
			name: "passthrough prefix default partition with subset",
			sni:  "passthrough~v2.api.default.dc1.internal." + td,
			want: UpstreamComponents{Service: "api", Namespace: "default", Partition: "default", Datacenter: "dc1"},
			ok:   true,
		},
		{
			name: "passthrough prefix non-default partition no subset",
			sni:  "passthrough~svc.ns1.partition-vms.dc1.internal-v1." + td,
			want: UpstreamComponents{Service: "svc", Namespace: "ns1", Partition: "partition-vms", Datacenter: "dc1"},
			ok:   true,
		},
		// passthrough + multi-port: both transformations apply together.
		{
			name: "passthrough prefix default partition multi-port",
			sni:  "passthrough~api-port.api.default.dc1.internal." + td,
			want: UpstreamComponents{Service: "api", Namespace: "default", Partition: "default", Datacenter: "dc1"},
			ok:   true,
		},

		// ── exported~ prefix (mesh-gateway exported clusters) ────────────────
		// meshGatewayExportedClusterNamePrefix = "exported~"
		{
			name: "exported prefix default partition no subset",
			sni:  "exported~api.default.dc1.internal." + td,
			want: UpstreamComponents{Service: "api", Namespace: "default", Partition: "default", Datacenter: "dc1"},
			ok:   true,
		},

		// ── customisation-hash prefix (naming.CustomizeClusterName) ──────────
		// CustomizeClusterName prepends "<8-hex>~" when the discovery chain
		// carries non-default customisation (protocol override, connect
		// timeout, etc.).  Single-port clusters are the tricky case: without
		// hash-stripping the parser would extract the hash as the service name.
		{
			name: "custom hash single-port default partition no subset",
			sni:  "f8f8f8f8~pong.default.dc2.internal." + td,
			want: UpstreamComponents{Service: "pong", Namespace: "default", Partition: "default", Datacenter: "dc2"},
			ok:   true,
		},
		{
			name: "custom hash single-port default partition with subset",
			sni:  "f8f8f8f8~v2.pong.default.dc2.internal." + td,
			want: UpstreamComponents{Service: "pong", Namespace: "default", Partition: "default", Datacenter: "dc2"},
			ok:   true,
		},
		{
			name: "custom hash single-port non-default partition no subset",
			sni:  "98809527~pong.ns1.partition-vms.dc2.internal-v1." + td,
			want: UpstreamComponents{Service: "pong", Namespace: "ns1", Partition: "partition-vms", Datacenter: "dc2"},
			ok:   true,
		},
		// Multi-port: the hash wraps the already-decorated cluster name.
		{
			name: "custom hash multi-port default partition",
			sni:  "f8f8f8f8~grpc-port.pong.default.dc2.internal." + td,
			want: UpstreamComponents{Service: "pong", Namespace: "default", Partition: "default", Datacenter: "dc2"},
			ok:   true,
		},
		// Combined: hash AND a literal prefix (e.g. passthrough inside a
		// customised transparent-proxy chain).
		{
			name: "custom hash combined with passthrough prefix",
			sni:  "f8f8f8f8~passthrough~api.default.dc1.internal." + td,
			want: UpstreamComponents{Service: "api", Namespace: "default", Partition: "default", Datacenter: "dc1"},
			ok:   true,
		},
		{
			name: "custom hash combined with exported prefix",
			sni:  "f8f8f8f8~exported~api.default.dc1.internal." + td,
			want: UpstreamComponents{Service: "api", Namespace: "default", Partition: "default", Datacenter: "dc1"},
			ok:   true,
		},
		// A non-hex token before "~" must NOT be stripped — the hash check
		// requires all 8 chars to be [0-9a-f].
		{
			name: "non-hex tilde prefix is not stripped",
			sni:  "zzzzzzzz~api.default.dc1.internal." + td,
			// "zzzzzzzz" is 8 chars but not valid hex, so it is not stripped.
			// The SNI is parsed as-is; "zzzzzzzz~api" becomes the first
			// dot-split label (tilde is not a dot), so the service extracted
			// by reading backwards from "internal" is "zzzzzzzz~api".
			want: UpstreamComponents{Service: "zzzzzzzz~api", Namespace: "default", Partition: "default", Datacenter: "dc1"},
			ok:   true,
		},

		// ── misc ──────────────────────────────────────────────────────────────
		{
			name: "trailing dot tolerated",
			sni:  "api.default.dc1.internal." + td + ".",
			want: UpstreamComponents{Service: "api", Namespace: "default", Partition: "default", Datacenter: "dc1"},
			ok:   true,
		},
		{
			name: "external not indexed",
			sni:  "web.default.default.peer1.external." + td,
			ok:   false,
		},
		{
			name: "empty",
			sni:  "",
			ok:   false,
		},
		{
			name: "unrelated fqdn",
			sni:  "example.com",
			ok:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseServiceSNI(tc.sni)
			if ok != tc.ok {
				t.Fatalf("ParseServiceSNI(%q) ok = %v, want %v", tc.sni, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("ParseServiceSNI(%q) = %+v, want %+v", tc.sni, got, tc.want)
			}
		})
	}
}

func TestUpstreamIndexLookup(t *testing.T) {
	const td = "e5b1a4d3.consul"
	idx := NewUpstreamIndex()
	idx.Update([]string{
		"service-response.default.partition-vms.dc1.internal-v1." + td,
		"service-response.default.partition-vms.dc2.internal-v1." + td,
		"api.default.dc1.internal." + td,
	}, nil)

	t.Run("unique by service", func(t *testing.T) {
		got, ok := idx.Lookup("api", "", "", "")
		if !ok {
			t.Fatal("expected match")
		}
		if got.Partition != "default" || got.Datacenter != "dc1" || got.Namespace != "default" {
			t.Fatalf("unexpected components: %+v", got)
		}
	})

	t.Run("ambiguous without constraint", func(t *testing.T) {
		if _, ok := idx.Lookup("service-response", "", "", ""); ok {
			t.Fatal("expected ambiguous lookup to fail")
		}
	})

	t.Run("disambiguated by datacenter", func(t *testing.T) {
		got, ok := idx.Lookup("service-response", "", "", "dc2")
		if !ok {
			t.Fatal("expected match")
		}
		if got.Partition != "partition-vms" || got.Datacenter != "dc2" {
			t.Fatalf("unexpected components: %+v", got)
		}
	})

	t.Run("unknown service", func(t *testing.T) {
		if _, ok := idx.Lookup("missing", "", "", ""); ok {
			t.Fatal("expected no match")
		}
	})

	t.Run("removal drops entry", func(t *testing.T) {
		idx.Update(nil, []string{"api.default.dc1.internal." + td})
		if _, ok := idx.Lookup("api", "", "", ""); ok {
			t.Fatal("expected entry to be removed")
		}
	})

	t.Run("nil index is safe", func(t *testing.T) {
		var nilIdx *UpstreamIndex
		nilIdx.Update([]string{"api.default.dc1.internal." + td}, nil)
		if _, ok := nilIdx.Lookup("api", "", "", ""); ok {
			t.Fatal("expected nil index to return no match")
		}
	})

	// ── multiport scenario ────────────────────────────────────────────────────
	// A multiport service produces one CDS cluster per named port, each with
	// the port prepended to the SNI: "<port>.<svc>.<ns>.<dc>.internal.<td>".
	// ParseServiceSNI strips the port label (it is skipped by the backward scan
	// from "internal"), so both clusters index under the same base service name.
	// Lookup("api", …) must therefore match using only the base service name,
	// which is what expandVirtualName passes after stripping the port from the
	// DNS query "http.api.virtual.consul".
	t.Run("multiport clusters index under base service name", func(t *testing.T) {
		mpIdx := NewUpstreamIndex()
		// Two named-port clusters for the same multiport service.
		mpIdx.Update([]string{
			"http.api.myns.myap.dc1.internal-v1." + td,
			"grpc.api.myns.myap.dc1.internal-v1." + td,
		}, nil)

		// Both ports must resolve to the same unique identity.
		got, ok := mpIdx.Lookup("api", "", "", "")
		if !ok {
			t.Fatal("expected multiport service to be found by base service name")
		}
		if got.Service != "api" || got.Namespace != "myns" || got.Partition != "myap" || got.Datacenter != "dc1" {
			t.Fatalf("unexpected components: %+v", got)
		}
	})

	t.Run("multiport clusters from different services are not conflated", func(t *testing.T) {
		mpIdx := NewUpstreamIndex()
		mpIdx.Update([]string{
			"http.svc-a.ns1.myap.dc1.internal-v1." + td,
			"http.svc-b.ns1.myap.dc1.internal-v1." + td,
		}, nil)

		// svc-a and svc-b are distinct services; each must resolve independently.
		gotA, okA := mpIdx.Lookup("svc-a", "", "", "")
		if !okA || gotA.Service != "svc-a" {
			t.Fatalf("expected svc-a to resolve, got ok=%v components=%+v", okA, gotA)
		}
		gotB, okB := mpIdx.Lookup("svc-b", "", "", "")
		if !okB || gotB.Service != "svc-b" {
			t.Fatalf("expected svc-b to resolve, got ok=%v components=%+v", okB, gotB)
		}
	})
}
