// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/hashicorp/consul-dataplane/pkg/dns"
)

// clusterTypeURL is the xDS type URL carried by CDS responses. Only responses
// with this type URL contribute upstream identities to the index.
const clusterTypeURL = "type.googleapis.com/envoy.config.cluster.v3.Cluster"

// Field numbers within envoy.service.discovery.v3.DeltaDiscoveryResponse.
// These mirror the generated proto (see go-control-plane discovery.pb.go) and
// are the raw wire identifiers we scan for so the dataplane does not need to
// pull in the full envoy proto dependency just to read cluster names.
const (
	deltaFieldResources        = 2 // repeated Resource
	deltaFieldTypeURL          = 4 // string
	deltaFieldRemovedResources = 6 // repeated string
	resourceFieldName          = 3 // string (the cluster SNI)
)

// tapClusterFrame inspects a single server->Envoy xDS frame flowing through the
// proxy. The proxy carries each frame as an opaque *emptypb.Empty whose unknown
// fields hold the wire-format DiscoveryResponse, so we read those raw bytes and
// hand-parse just the cluster names. When the frame is a CDS response, the
// added and removed cluster SNIs are applied to the upstream index. Any other
// frame (or an unparseable one) is ignored; the frame itself is never modified,
// so passthrough is unaffected.
func tapClusterFrame(index *dns.UpstreamIndex, m interface{}) {
	if index == nil {
		return
	}
	empty, ok := m.(*emptypb.Empty)
	if !ok {
		return
	}
	raw := empty.ProtoReflect().GetUnknown()
	if len(raw) == 0 {
		return
	}
	added, removed, isCluster := extractClusterSNIs(raw)
	if !isCluster {
		return
	}
	index.Update(added, removed)
}

// extractClusterSNIs scans a wire-format DeltaDiscoveryResponse for the cluster
// SNIs it added/updated and removed. It returns isCluster=false (and no names)
// when the frame is not a CDS response or cannot be parsed.
func extractClusterSNIs(raw []byte) (added, removed []string, isCluster bool) {
	var (
		typeURL string
		names   []string
		gone    []string
	)
	b := raw
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, nil, false
		}
		b = b[n:]
		switch {
		case num == deltaFieldTypeURL && typ == protowire.BytesType:
			v, vn := protowire.ConsumeBytes(b)
			if vn < 0 {
				return nil, nil, false
			}
			typeURL = string(v)
			b = b[vn:]
		case num == deltaFieldResources && typ == protowire.BytesType:
			v, vn := protowire.ConsumeBytes(b)
			if vn < 0 {
				return nil, nil, false
			}
			if name, ok := extractResourceName(v); ok {
				names = append(names, name)
			}
			b = b[vn:]
		case num == deltaFieldRemovedResources && typ == protowire.BytesType:
			v, vn := protowire.ConsumeBytes(b)
			if vn < 0 {
				return nil, nil, false
			}
			gone = append(gone, string(v))
			b = b[vn:]
		default:
			vn := protowire.ConsumeFieldValue(num, typ, b)
			if vn < 0 {
				return nil, nil, false
			}
			b = b[vn:]
		}
	}
	if typeURL != clusterTypeURL {
		return nil, nil, false
	}
	return names, gone, true
}

// extractResourceName scans a wire-format DeltaDiscoveryResponse.Resource for
// its name field, which for a CDS cluster is the upstream SNI.
func extractResourceName(raw []byte) (string, bool) {
	b := raw
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return "", false
		}
		b = b[n:]
		if num == resourceFieldName && typ == protowire.BytesType {
			v, vn := protowire.ConsumeBytes(b)
			if vn < 0 {
				return "", false
			}
			return string(v), true
		}
		vn := protowire.ConsumeFieldValue(num, typ, b)
		if vn < 0 {
			return "", false
		}
		b = b[vn:]
	}
	return "", false
}
