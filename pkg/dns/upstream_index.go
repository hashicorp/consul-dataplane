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
)

// ParseServiceSNI parses a Consul upstream service SNI into its identity
// components. It understands the internal (mesh) SNI schemes emitted by the
// Consul control plane:
//
//	default partition, no subset:   <svc>.<ns>.<dc>.internal.<trustdomain>
//	default partition, with subset: <subset>.<svc>.<ns>.<dc>.internal.<trustdomain>
//	other partition, no subset:     <svc>.<ns>.<ap>.<dc>.internal-v1.<trustdomain>
//	other partition, with subset:   <subset>.<svc>.<ns>.<ap>.<dc>.internal-v1.<trustdomain>
//
// The subset label, when present, is ignored: virtual-FQDN expansion keys off
// the base service name, and every subset of a service shares its identity.
//
// This parsing is intentionally coupled to Consul's SNI label scheme (see
// agent/connect/sni.go in the Consul server). SNIs that are external, peered,
// gateway, or prepared-query are not internal upstreams and return ok=false.
func ParseServiceSNI(sni string) (UpstreamComponents, bool) {
	sni = strings.ToLower(strings.TrimSuffix(sni, "."))
	if sni == "" {
		return UpstreamComponents{}, false
	}
	labels := strings.Split(sni, ".")

	// Locate the scheme marker. internal-v1 is checked before internal because
	// it is the more specific token.
	for i, label := range labels {
		switch label {
		case sniMarkerInternalV1:
			// prefix = labels[:i]
			switch i {
			case 4: // svc, ns, ap, dc
				return UpstreamComponents{
					Service:    labels[0],
					Namespace:  labels[1],
					Partition:  labels[2],
					Datacenter: labels[3],
				}, true
			case 5: // subset, svc, ns, ap, dc
				return UpstreamComponents{
					Service:    labels[1],
					Namespace:  labels[2],
					Partition:  labels[3],
					Datacenter: labels[4],
				}, true
			}
		case sniMarkerInternal:
			switch i {
			case 3: // svc, ns, dc (default partition)
				return UpstreamComponents{
					Service:    labels[0],
					Namespace:  labels[1],
					Partition:  "default",
					Datacenter: labels[2],
				}, true
			case 4: // subset, svc, ns, dc (default partition)
				return UpstreamComponents{
					Service:    labels[1],
					Namespace:  labels[2],
					Partition:  "default",
					Datacenter: labels[3],
				}, true
			}
		}
	}
	return UpstreamComponents{}, false
}
