// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package credentialbroker

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

func peerUID(conn net.Conn) (uint32, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("not a unix connection")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("SO_PEERCRED SyscallConn: %w", err)
	}
	var uid uint32
	var ctrlErr error
	if err := raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			ctrlErr = err
			return
		}
		uid = cred.Uid
	}); err != nil {
		return 0, fmt.Errorf("SO_PEERCRED Control: %w", err)
	}
	if ctrlErr != nil {
		return 0, fmt.Errorf("SO_PEERCRED: %w", ctrlErr)
	}
	return uid, nil
}
