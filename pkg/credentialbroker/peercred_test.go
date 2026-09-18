// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build linux || darwin

package credentialbroker

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestPeerCredAcceptsSameUID(t *testing.T) {
	path := unixTestSock(t)
	lis, err := net.Listen("unix", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })
	wrapped := wrapPeerCreds(lis, hclog.NewNullLogger())

	errCh := make(chan error, 1)
	go func() {
		c, err := wrapped.Accept()
		if err != nil {
			errCh <- err
			return
		}
		_ = c.Close()
		errCh <- nil
	}()

	conn, err := net.Dial("unix", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Accept did not return")
	}
}

func TestPeerCredRejectsMismatchedUID(t *testing.T) {
	path := unixTestSock(t)
	lis, err := net.Listen("unix", path)
	require.NoError(t, err)
	wrapped := wrapPeerCreds(lis, hclog.NewNullLogger()).(*peerCredListener)
	wrapped.uid = wrapped.uid + 1

	errCh := make(chan error, 1)
	go func() {
		_, err := wrapped.Accept()
		errCh <- err
	}()

	conn, err := net.Dial("unix", path)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, conn)
	_ = conn.Close()

	require.NoError(t, lis.Close())
	select {
	case err := <-errCh:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Accept did not return after listener close")
	}
}

func TestPeerUIDRejectsNonUnix(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err == nil {
			defer c.Close()
			_, _ = io.Copy(io.Discard, c)
		}
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = peerUID(conn)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a unix connection")
}

func TestWrapPeerCredsNilLogger(t *testing.T) {
	path := unixTestSock(t)
	lis, err := net.Listen("unix", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })
	wrapped := wrapPeerCreds(lis, nil)
	require.NotNil(t, wrapped)
}

func unixTestSock(t *testing.T) string {
	t.Helper()
	path := filepath.Join("/tmp", filepath.Base(t.TempDir())+"-peercred.sock")
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}
