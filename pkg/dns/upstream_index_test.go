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
}
