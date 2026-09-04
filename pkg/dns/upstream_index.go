// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package dns

import (
	"strings"
	"sync"
)

// UpstreamComponents captures the real identity of an upstream service as
// advertised by the control plane through the upstream cluster SNI. These are
// the raw tokens the control plane itself used to build Envoy's inline DNS
// table, so filling missing virtual-FQDN tokens from them keeps the dataplane's
// expansion consistent with what Envoy will actually answer.
type UpstreamComponents struct {
	Service    string
	Namespace  string
	Partition  string
	Datacenter string
}

// UpstreamIndex is a thread-safe index of upstream identities keyed by their
// SNI. It is populated by decoding the CDS (Cluster) resources that already
// flow through the dataplane's proxied xDS stream, so it introduces no new
// watch and no control-plane change. The last decoded state is retained across
// xDS reconnects so virtual expansion keeps working during a control-plane
// outage (BCDR resilience).
type UpstreamIndex struct {
	mu sync.RWMutex
	// entries is keyed by the full SNI (the CDS resource name) so that delta
	// removals can drop exactly the upstream that went away.
	entries map[string]UpstreamComponents
}

// NewUpstreamIndex returns an empty, ready-to-use UpstreamIndex.
func NewUpstreamIndex() *UpstreamIndex {
	return &UpstreamIndex{
		entries: make(map[string]UpstreamComponents),
	}
}

// Update applies a delta of CDS cluster SNIs to the index. added holds SNIs of
// clusters that were added or modified; removed holds SNIs of clusters that
// were removed. Non-internal SNIs (external, peered, gateway, prepared-query)
// are ignored because they do not participate in virtual-FQDN expansion.
func (idx *UpstreamIndex) Update(added, removed []string) {
	if idx == nil {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	for _, sni := range removed {
		delete(idx.entries, sni)
	}
	for _, sni := range added {
		comp, ok := ParseServiceSNI(sni)
		if !ok {
			continue
		}
		idx.entries[sni] = comp
	}
}

// Lookup resolves the identity of the named service, using any explicitly
// provided namespace/partition/datacenter tokens as constraints. Empty
// constraint tokens match any value. It returns the matched components only
// when the constraints resolve to a single, unambiguous identity; otherwise it
// returns false so the caller can fall back to the source proxy's defaults.
func (idx *UpstreamIndex) Lookup(service, namespace, partition, datacenter string) (UpstreamComponents, bool) {
	if idx == nil || service == "" {
		return UpstreamComponents{}, false
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var (
		match UpstreamComponents
		found bool
	)
	for _, comp := range idx.entries {
		if comp.Service != service {
			continue
		}
		if namespace != "" && comp.Namespace != namespace {
			continue
		}
		if partition != "" && comp.Partition != partition {
			continue
		}
		if datacenter != "" && comp.Datacenter != datacenter {
			continue
		}
		if found && comp != match {
			// More than one distinct identity satisfies the constraints;
			// the query is ambiguous, so refuse to guess.
			return UpstreamComponents{}, false
		}
		match = comp
		found = true
	}
	return match, found
}

const (
	sniMarkerInternal   = "internal"
	sniMarkerInternalV1 = "internal-v1"

	// customizationHashLen is the exact length of the hex string that
	// naming.CustomizeClusterName (agent/xds/naming/naming.go) prepends when a
	// discovery-chain has non-default customisation (e.g. protocol overrides).
	// Consul always formats it as fmt.Sprintf("%x", v)[0:8] — eight lower-case
	// hexadecimal characters — followed by "~".
	customizationHashLen = 8
)

// sniClusterPrefixes lists the non-SNI prefixes that the Consul control plane
// prepends to a bare service SNI when naming special Envoy clusters:
//
//   - "passthrough~" — transparent-proxy passthrough clusters
//     (agent/xds/clusters.go: name = "passthrough~" + sni)
//   - "exported~"    — mesh-gateway exported clusters
//     (agent/xds/clusters.go: meshGatewayExportedClusterNamePrefix)
//
// These strings appear as the CDS resource name (and therefore as the value
// received by Update), but they are not part of the SNI itself. Stripping
// them before parsing keeps the index-key and SNI-label counts consistent.
var sniClusterPrefixes = []string{"passthrough~", "exported~"}

// stripClusterNamePrefixes removes the zero, one, or two "~"-delimited
// cluster-name tokens that the Consul control plane may prepend to a bare
// service SNI.  In production the prefixes arrive in this order (outermost
// first):
//
//  1. An 8-hex-char customisation hash added by naming.CustomizeClusterName,
//     e.g. "f8f8f8f8~".  Present only when the discovery chain has non-default
//     customisation (protocol override, connect timeout, etc.).
//  2. A literal prefix such as "passthrough~" or "exported~".
//
// A single pass over each layer is enough; they do not nest further.
func stripClusterNamePrefixes(sni string) string {
	// Layer 1 – customisation hash: <8 lower-case hex chars> "~"
	// The SNI was already lowercased by the caller, so valid chars are [0-9a-f].
	if len(sni) > customizationHashLen+1 && sni[customizationHashLen] == '~' {
		allHex := true
		for i := 0; i < customizationHashLen; i++ {
			c := sni[i]
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				allHex = false
				break
			}
		}
		if allHex {
			sni = sni[customizationHashLen+1:]
		}
	}
	// Layer 2 – literal cluster-name prefix (passthrough~, exported~, …)
	for _, pfx := range sniClusterPrefixes {
		if strings.HasPrefix(sni, pfx) {
			sni = sni[len(pfx):]
			break
		}
	}
	return sni
}

// ParseServiceSNI parses a Consul upstream service SNI into its identity
// components. It understands the internal (mesh) SNI schemes emitted by the
// Consul control plane:
//
//	default partition, no subset:            <svc>.<ns>.<dc>.internal.<trustdomain>
//	default partition, with subset:          <subset>.<svc>.<ns>.<dc>.internal.<trustdomain>
//	default partition, multi-port:           <port>.<svc>.<ns>.<dc>.internal.<trustdomain>
//	default partition, multi-port+subset:    <port>.<subset>.<svc>.<ns>.<dc>.internal.<trustdomain>
//	non-default partition, no subset:        <svc>.<ns>.<ap>.<dc>.internal-v1.<trustdomain>
//	non-default partition, with subset:      <subset>.<svc>.<ns>.<ap>.<dc>.internal-v1.<trustdomain>
//	non-default partition, multi-port:       <port>.<svc>.<ns>.<ap>.<dc>.internal-v1.<trustdomain>
//	non-default partition, multi-port+subset:<port>.<subset>.<svc>.<ns>.<ap>.<dc>.internal-v1.<trustdomain>
//
// Any of the above may arrive prefixed with a customisation hash
// (naming.CustomizeClusterName), a literal cluster-name token such as
// "passthrough~" or "exported~", or both in that order.  All such prefixes
// are stripped before parsing.
//
// Both the subset label and the port-name label, when present, are ignored:
// virtual-FQDN expansion keys off the base service identity only, and every
// subset/port of a service shares that identity.
//
// This parsing is intentionally coupled to Consul's SNI label scheme (see
// agent/connect/sni.go in the Consul server). SNIs that are external, peered,
// gateway, or prepared-query are not internal upstreams and return ok=false.
func ParseServiceSNI(sni string) (UpstreamComponents, bool) {
	sni = strings.ToLower(strings.TrimSuffix(sni, "."))
	if sni == "" {
		return UpstreamComponents{}, false
	}

	sni = stripClusterNamePrefixes(sni)

	labels := strings.Split(sni, ".")
	// Scan for the scheme marker. Once found, the identity labels immediately
	// precede it in fixed relative positions — read backwards from i so that
	// any number of leading port/subset labels are automatically skipped
	// without enumerating absolute indices.
	//
	// internal-v1:  labels[i-4]=svc  labels[i-3]=ns  labels[i-2]=ap  labels[i-1]=dc
	// internal:     labels[i-3]=svc  labels[i-2]=ns                   labels[i-1]=dc
	//
	// i must be large enough that all four (or three) positions are in-bounds.
	for i, label := range labels {
		switch label {
		case sniMarkerInternalV1:
			// Need at least 4 labels before the marker: svc, ns, ap, dc.
			if i < 4 {
				continue
			}
			return UpstreamComponents{
				Service:    labels[i-4],
				Namespace:  labels[i-3],
				Partition:  labels[i-2],
				Datacenter: labels[i-1],
			}, true
		case sniMarkerInternal:
			// Need at least 3 labels before the marker: svc, ns, dc.
			if i < 3 {
				continue
			}
			return UpstreamComponents{
				Service:    labels[i-3],
				Namespace:  labels[i-2],
				Partition:  "default",
				Datacenter: labels[i-1],
			}, true
		}
	}
	return UpstreamComponents{}, false
}
