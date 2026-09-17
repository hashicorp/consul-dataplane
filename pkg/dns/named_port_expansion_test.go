// Copyright IBM Corp. 2023, 2026
// SPDX-License-Identifier: MPL-2.0

package dns

import "testing"

// A multi-port service gives every named port its own virtual IP, and the
// "<port>.<svc>.virtual..." name selects one of them.
//
// Envoy's inline DNS table is keyed on the base service name, so it cannot
// answer a named-port query; expandVirtualName deliberately drops the port
// label to match that keying. Forwarding such a query to the inline listener
// therefore asks for the *service*, and a hit answers with the service-level
// virtual IP -- which routes to the service's default port, handing the caller
// another port's response with a 200 rather than an error.
//
// The proxy now detects a named port and queries Consul directly, which
// resolves the port label itself. parseVirtualTokens reporting the port is the
// predicate that decision rests on, so it is pinned here for every form the
// classifier accepts.
//
// This only ever went wrong when the inline listener answered: on a miss the
// proxy already fell back to Consul with the original, unexpanded query, where
// the port label is intact. That is why it reproduced in CI but not locally.
func TestParseVirtualTokens_ReportsNamedPort(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantPort string
		wantSvc  string
	}{
		{
			name:     "named port with full qualifiers",
			input:    "api-port.multiport.virtual.default.ns.default.ap.dc1.dc.consul",
			wantPort: "api-port",
			wantSvc:  "multiport",
		},
		{
			name:     "named port bare form",
			input:    "metrics.api.virtual.consul",
			wantPort: "metrics",
			wantSvc:  "api",
		},
		{
			name:     "named port with service alias",
			input:    "http.api.service.virtual.consul",
			wantPort: "http",
			wantSvc:  "api",
		},
		{
			name:     "named port with partial qualifiers",
			input:    "grpc.api.virtual.myns.ns.consul",
			wantPort: "grpc",
			wantSvc:  "api",
		},
		{
			name:     "service-level name reports no port",
			input:    "multiport.virtual.default.ns.default.ap.dc1.dc.consul",
			wantPort: "",
			wantSvc:  "multiport",
		},
		{
			name:     "service-level bare form reports no port",
			input:    "api.virtual.consul",
			wantPort: "",
			wantSvc:  "api",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			port, svc, _, _, _, ok := parseVirtualTokens(tc.input)
			if !ok {
				t.Fatalf("parseVirtualTokens(%q) returned ok=false", tc.input)
			}
			if port != tc.wantPort {
				t.Errorf("port: got %q, want %q", port, tc.wantPort)
			}
			if svc != tc.wantSvc {
				t.Errorf("svc: got %q, want %q", svc, tc.wantSvc)
			}
		})
	}
}

// expandVirtualName still collapses a named-port name onto the base service,
// because that is what the inline table is keyed on. Pinned here so the
// relationship between the two behaviours stays explicit: the port label is
// dropped during expansion precisely because named-port queries must not reach
// the inline listener at all.
func TestExpandVirtualName_StillCollapsesNamedPort(t *testing.T) {
	d := &DNSServer{namespace: "default", partition: "default", datacenter: "dc1"}

	got := d.expandVirtualName("metrics.multiport.virtual.consul")
	want := "multiport.virtual.default.ns.default.ap.dc1.dc.consul"
	if got != want {
		t.Errorf("expandVirtualName\n got: %q\nwant: %q", got, want)
	}
}
