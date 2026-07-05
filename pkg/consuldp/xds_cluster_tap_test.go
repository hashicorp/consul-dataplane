// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/hashicorp/consul-dataplane/pkg/dns"
)

func encodeResource(name string) []byte {
	var b []byte
	b = protowire.AppendTag(b, resourceFieldName, protowire.BytesType)
	b = protowire.AppendBytes(b, []byte(name))
	return b
}

func encodeDeltaResponse(typeURL string, names, removed []string) []byte {
	var b []byte
	b = protowire.AppendTag(b, deltaFieldTypeURL, protowire.BytesType)
	b = protowire.AppendBytes(b, []byte(typeURL))
	for _, n := range names {
		b = protowire.AppendTag(b, deltaFieldResources, protowire.BytesType)
		b = protowire.AppendBytes(b, encodeResource(n))
	}
	for _, r := range removed {
		b = protowire.AppendTag(b, deltaFieldRemovedResources, protowire.BytesType)
		b = protowire.AppendBytes(b, []byte(r))
	}
	return b
}

func emptyWithBytes(b []byte) *emptypb.Empty {
	e := &emptypb.Empty{}
	e.ProtoReflect().SetUnknown(protoreflect.RawFields(b))
	return e
}

func TestExtractClusterSNIs(t *testing.T) {
	const td = "e5b1a4d3.consul"

	t.Run("cluster response", func(t *testing.T) {
		raw := encodeDeltaResponse(clusterTypeURL,
			[]string{"api.default.dc1.internal." + td},
			[]string{"web.default.partition-vms.dc1.internal-v1." + td},
		)
		added, removed, ok := extractClusterSNIs(raw)
		if !ok {
			t.Fatal("expected cluster response")
		}
		if len(added) != 1 || added[0] != "api.default.dc1.internal."+td {
			t.Fatalf("unexpected added: %v", added)
		}
		if len(removed) != 1 || removed[0] != "web.default.partition-vms.dc1.internal-v1."+td {
			t.Fatalf("unexpected removed: %v", removed)
		}
	})

	t.Run("non-cluster response ignored", func(t *testing.T) {
		raw := encodeDeltaResponse(
			"type.googleapis.com/envoy.config.listener.v3.Listener",
			[]string{"ignored"}, nil)
		if _, _, ok := extractClusterSNIs(raw); ok {
			t.Fatal("expected non-cluster response to be ignored")
		}
	})

	t.Run("empty bytes", func(t *testing.T) {
		if _, _, ok := extractClusterSNIs(nil); ok {
			t.Fatal("expected empty input to be ignored")
		}
	})
}

func TestTapClusterFrame(t *testing.T) {
	const td = "e5b1a4d3.consul"
	idx := dns.NewUpstreamIndex()

	raw := encodeDeltaResponse(clusterTypeURL,
		[]string{"api.default.partition-vms.dc1.internal-v1." + td}, nil)
	tapClusterFrame(idx, emptyWithBytes(raw))

	if _, ok := idx.Lookup("api", "", "partition-vms", "dc1"); !ok {
		t.Fatal("expected cluster SNI to be indexed")
	}

	// Non-Empty message must be ignored without panicking.
	tapClusterFrame(idx, "not-an-empty")

	// A removal frame drops the entry.
	rm := encodeDeltaResponse(clusterTypeURL, nil,
		[]string{"api.default.partition-vms.dc1.internal-v1." + td})
	tapClusterFrame(idx, emptyWithBytes(rm))
	if _, ok := idx.Lookup("api", "", "partition-vms", "dc1"); ok {
		t.Fatal("expected entry to be removed")
	}
}
