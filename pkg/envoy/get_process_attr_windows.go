// Copyright IBM Corp. 2022, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package envoy

import "syscall"

func getProcessAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
