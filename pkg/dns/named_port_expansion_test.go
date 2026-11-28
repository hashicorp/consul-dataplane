// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package dns

import "testing"

// The control plane keys Envoy's inline DNS table on every label that tells
// upstream identities apart: the named port of a multiport service and the peer
// of an imported service. expandVirtualName must produce that same key, keeping
// the port and peer labels when present and omitting them otherwise.
func TestExpandVirtualName(t *testing.T) {
	d := &DNSServer{namespace: "default", partition: "default", datacenter: "dc1"}

	cases := map[string]string{
		"api.virtual.consul":                                     "api.virtual.default.ns.default.ap.dc1.dc.consul",
		"metrics.multiport.virtual.consul":                       "metrics.multiport.virtual.default.ns.default.ap.dc1.dc.consul",
		"db.virtual.peer-a.peer.consul":                          "db.virtual.peer-a.peer.default.ns.default.ap.dc1.dc.consul",
		"grpc.db.virtual.peer-a.peer.myns.ns.consul":             "grpc.db.virtual.peer-a.peer.myns.ns.default.ap.dc1.dc.consul",
		"api-port.multiport.virtual.ns1.ns.ap1.ap.dc2.dc.consul": "api-port.multiport.virtual.ns1.ns.ap1.ap.dc2.dc.consul",
	}
	for in, want := range cases {
		if got := d.expandVirtualName(in); got != want {
			t.Errorf("expandVirtualName(%q)\n got: %q\nwant: %q", in, got, want)
		}
	}
}

func TestParseVirtualTokens_ReportsNamedPort(t *testing.T) {
	cases := map[string]string{
		"api-port.multiport.virtual.default.ns.default.ap.dc1.dc.consul": "api-port",
		"metrics.api.virtual.consul":                                     "metrics",
		"api.virtual.consul":                                             "",
	}
	for in, wantPort := range cases {
		got, ok := parseVirtualTokens(in)
		if !ok {
			t.Fatalf("parseVirtualTokens(%q) returned ok=false", in)
		}
		if got.port != wantPort {
			t.Errorf("parseVirtualTokens(%q) port: got %q, want %q", in, got.port, wantPort)
		}
	}
}
