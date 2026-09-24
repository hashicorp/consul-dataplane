// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build linux || darwin

package credentialbroker

import (
	"net"
	"os"

	"github.com/hashicorp/go-hclog"
)

type peerCredListener struct {
	net.Listener
	uid    uint32
	logger hclog.Logger
}

func wrapPeerCreds(lis net.Listener, logger hclog.Logger) net.Listener {
	if logger == nil {
		logger = hclog.NewNullLogger()
	}
	return &peerCredListener{Listener: lis, uid: uint32(os.Getuid()), logger: logger}
}

func (l *peerCredListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		uid, err := peerUID(c)
		if err != nil {
			_ = c.Close()
			l.logger.Warn("credential broker rejected connection", "error", err)
			continue
		}
		if uid != l.uid {
			_ = c.Close()
			l.logger.Warn("credential broker rejected connection", "peer_uid", uid)
			continue
		}
		return c, nil
	}
}
