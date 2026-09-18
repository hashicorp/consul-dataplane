// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build !linux && !darwin

package credentialbroker

import (
	"net"

	"github.com/hashicorp/go-hclog"
)

func wrapPeerCreds(lis net.Listener, _ hclog.Logger) net.Listener {
	return lis
}
