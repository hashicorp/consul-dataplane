// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package credentialbroker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/hashicorp/consul-dataplane/internal/pbdataplane"
)

type staticFetcher struct {
	resp *pbdataplane.FetchKeyResponse
	err  error
}

func (f staticFetcher) FetchKey(context.Context, string) (*pbdataplane.FetchKeyResponse, error) {
	return f.resp, f.err
}

func TestBrokerGetKeyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join("/tmp", filepath.Base(dir)+"-cb.sock")
	t.Cleanup(func() { _ = os.Remove(path) })
	dek := []byte("0123456789abcdef0123456789abcdef")
	now := time.Now()
	fetcher := staticFetcher{resp: &pbdataplane.FetchKeyResponse{
		KeyId:            "k43",
		KeyMaterial:      dek,
		Algorithm:        "AES-256-GCM",
		NotBeforeUnix:    now.Add(-time.Minute).Unix(),
		RefreshAfterUnix: now.Add(time.Hour).Unix(),
		ExpiresAtUnix:    now.Add(2 * time.Hour).Unix(),
	}}
	b := New(Config{BindAddr: "unix://" + path, Fetcher: fetcher})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- b.Serve(ctx) }()

	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, 2*time.Second, 20*time.Millisecond)

	conn, err := grpc.NewClient("unix://"+path, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := pbdataplane.NewLocalCredentialBrokerClient(conn)
	got, err := client.GetKey(ctx, &pbdataplane.GetKeyRequest{KeyId: "k43"})
	require.NoError(t, err)
	require.Equal(t, dek, got.KeyMaterial)
	require.Equal(t, "k43", got.KeyId)

	st, err := client.GetBrokerStatus(ctx, &pbdataplane.GetBrokerStatusRequest{})
	require.NoError(t, err)
	require.True(t, st.Ready)
	require.Equal(t, int32(1), st.KeysCached)
}

func TestBrokerExpiredKeyFailsClosed(t *testing.T) {
	b := New(Config{Fetcher: staticFetcher{resp: &pbdataplane.FetchKeyResponse{
		KeyId:         "k",
		KeyMaterial:   []byte("0123456789abcdef0123456789abcdef"),
		ExpiresAtUnix: time.Now().Add(-time.Second).Unix(),
	}}})
	_, err := b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k"})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestBrokerMissingKeyIDNotFound(t *testing.T) {
	b := New(Config{Fetcher: staticFetcher{err: status.Error(codes.NotFound, "not found")}})
	_, err := b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{})
	require.Equal(t, codes.NotFound, status.Code(err))
	_, err = b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "missing"})
	require.Equal(t, codes.NotFound, status.Code(err))
}

type countingFetcher struct {
	mu   sync.Mutex
	n    int
	resp *pbdataplane.FetchKeyResponse
	err  error
}

func (f *countingFetcher) FetchKey(context.Context, string) (*pbdataplane.FetchKeyResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	return f.resp, f.err
}

func (f *countingFetcher) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

func TestCacheFromFetchDefaultRefreshFraction(t *testing.T) {
	rec, err := cacheFromFetch(&pbdataplane.FetchKeyResponse{
		NotBeforeUnix: 1000,
		ExpiresAtUnix: 2000,
	}, 0.2)
	require.NoError(t, err)
	require.Equal(t, time.Unix(1800, 0), rec.refreshAfter)
	require.Equal(t, time.Unix(2000, 0), rec.expiresAt)
}

func TestCacheFromFetchNilResponse(t *testing.T) {
	_, err := cacheFromFetch(nil, 0.2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil FetchKeyResponse")
}

func TestBrokerNilFetcherRejected(t *testing.T) {
	b := New(Config{BindAddr: "unix:///tmp/unused-cb.sock"})
	err := b.Start(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "Fetcher is required")

	_, err = b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k"})
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestBrokerNilFetchResponseNotFound(t *testing.T) {
	b := New(Config{Fetcher: staticFetcher{resp: nil}})
	_, err := b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k"})
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestBrokerExpiredRefreshKeepsValidCache(t *testing.T) {
	now := time.Now()
	fetcher := &countingFetcher{resp: &pbdataplane.FetchKeyResponse{
		KeyId:            "k43",
		KeyMaterial:      []byte("0123456789abcdef0123456789abcdef"),
		NotBeforeUnix:    now.Add(-time.Minute).Unix(),
		RefreshAfterUnix: now.Add(-time.Second).Unix(),
		ExpiresAtUnix:    now.Add(time.Hour).Unix(),
	}}
	b := New(Config{Fetcher: fetcher})
	got, err := b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k43"})
	require.NoError(t, err)
	require.Equal(t, []byte("0123456789abcdef0123456789abcdef"), got.KeyMaterial)

	fetcher.resp = &pbdataplane.FetchKeyResponse{
		KeyId:         "k43",
		KeyMaterial:   []byte("expired-key-material-should-not!!"),
		ExpiresAtUnix: now.Add(-time.Second).Unix(),
	}
	got, err = b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k43"})
	require.NoError(t, err)
	require.Equal(t, []byte("0123456789abcdef0123456789abcdef"), got.KeyMaterial)
	require.Equal(t, 2, fetcher.calls())
}

type blockingFetcher struct {
	started chan struct{}
	release chan struct{}
	resp    *pbdataplane.FetchKeyResponse
	n       int
	mu      sync.Mutex
}

func (f *blockingFetcher) FetchKey(context.Context, string) (*pbdataplane.FetchKeyResponse, error) {
	f.mu.Lock()
	f.n++
	n := f.n
	f.mu.Unlock()
	if n == 1 {
		close(f.started)
		<-f.release
	}
	return f.resp, nil
}

func (f *blockingFetcher) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

func TestBrokerRefreshSingleflight(t *testing.T) {
	now := time.Now()
	fetcher := &blockingFetcher{
		started: make(chan struct{}),
		release: make(chan struct{}),
		// Refresh result must be fresh. A still-due refreshAfter lets callers
		// that miss the in-flight call start another FetchKey.
		resp: &pbdataplane.FetchKeyResponse{
			KeyId:            "k43",
			KeyMaterial:      []byte("0123456789abcdef0123456789abcdef"),
			RefreshAfterUnix: now.Add(time.Hour).Unix(),
			ExpiresAtUnix:    now.Add(2 * time.Hour).Unix(),
		},
	}
	b := New(Config{Fetcher: fetcher})
	// Seed cache already past refreshAfter so concurrent Gets coalesce.
	b.put("k43", cacheFromFetchMust(t, &pbdataplane.FetchKeyResponse{
		KeyId:            "k43",
		KeyMaterial:      []byte("0123456789abcdef0123456789abcdef"),
		RefreshAfterUnix: now.Add(-time.Second).Unix(),
		ExpiresAtUnix:    now.Add(time.Hour).Unix(),
	}, 0.2))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k43"})
			require.NoError(t, err)
		}()
	}
	select {
	case <-fetcher.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for FetchKey")
	}
	// Hold the fetch so the other callers join it before it finishes.
	time.Sleep(50 * time.Millisecond)
	close(fetcher.release)
	wg.Wait()
	require.Equal(t, 1, fetcher.calls())
}

func cacheFromFetchMust(t *testing.T, resp *pbdataplane.FetchKeyResponse, fraction float64) *cachedKey {
	t.Helper()
	rec, err := cacheFromFetch(resp, fraction)
	require.NoError(t, err)
	return rec
}

func TestBrokerRefreshFetchesAtTwentyPercentRemaining(t *testing.T) {
	now := time.Now()
	fetcher := &countingFetcher{resp: &pbdataplane.FetchKeyResponse{
		KeyId:            "k43",
		KeyMaterial:      []byte("0123456789abcdef0123456789abcdef"),
		NotBeforeUnix:    now.Add(-time.Minute).Unix(),
		RefreshAfterUnix: now.Add(time.Hour).Unix(),
		ExpiresAtUnix:    now.Add(2 * time.Hour).Unix(),
	}}
	b := New(Config{Fetcher: fetcher})
	_, err := b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k43"})
	require.NoError(t, err)
	_, err = b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k43"})
	require.NoError(t, err)
	require.Equal(t, 1, fetcher.calls())

	fetcher.resp.RefreshAfterUnix = now.Add(-time.Second).Unix()
	b.put("k43", cacheFromFetchMust(t, fetcher.resp, 0.2))
	_, err = b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k43"})
	require.NoError(t, err)
	require.Equal(t, 2, fetcher.calls())
}

func TestBrokerRestartRefetches(t *testing.T) {
	now := time.Now()
	fetcher := &countingFetcher{resp: &pbdataplane.FetchKeyResponse{
		KeyId:         "k43",
		KeyMaterial:   []byte("0123456789abcdef0123456789abcdef"),
		ExpiresAtUnix: now.Add(time.Hour).Unix(),
	}}
	b1 := New(Config{Fetcher: fetcher})
	_, err := b1.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k43"})
	require.NoError(t, err)
	b2 := New(Config{Fetcher: fetcher})
	_, err = b2.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k43"})
	require.NoError(t, err)
	require.Equal(t, 2, fetcher.calls())
}

func TestBrokerDoesNotLogKeyMaterial(t *testing.T) {
	var buf bytes.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "credential-broker",
		Output: &buf,
		Level:  hclog.Trace,
	})
	dek := []byte("super-secret-dek-material-32b!!")
	now := time.Now()
	b := New(Config{
		Logger: logger,
		Fetcher: staticFetcher{resp: &pbdataplane.FetchKeyResponse{
			KeyId:         "k43",
			KeyMaterial:   dek,
			ExpiresAtUnix: now.Add(time.Hour).Unix(),
		}},
	})
	_, err := b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k43"})
	require.NoError(t, err)
	logs := buf.String()
	require.NotContains(t, logs, string(dek))
	require.NotContains(t, logs, fmt.Sprintf("%x", dek))
}

func TestUnixSocketPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		bind    string
		want    string
		wantErr string
	}{
		{name: "empty", bind: "", want: ""},
		{name: "unix url", bind: "unix:///consul/connect-inject/credential-broker.sock", want: "/consul/connect-inject/credential-broker.sock"},
		{name: "bare path", bind: "/consul/connect-inject/credential-broker.sock", want: "/consul/connect-inject/credential-broker.sock"},
		{name: "empty unix url", bind: "unix://", wantErr: "unix socket path is empty"},
		{name: "tcp url", bind: "tcp://127.0.0.1:8080", wantErr: "must be a unix socket"},
		{name: "host port", bind: "127.0.0.1:8080", wantErr: "must be a unix socket"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := unixSocketPath(tt.bind)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestStartBindsSocketBeforeReturn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join("/tmp", filepath.Base(dir)+"-cb-start.sock")
	t.Cleanup(func() { _ = os.Remove(path) })
	now := time.Now()
	b := New(Config{
		BindAddr: "unix://" + path,
		Fetcher: staticFetcher{resp: &pbdataplane.FetchKeyResponse{
			KeyId:         "k43",
			KeyMaterial:   []byte("0123456789abcdef0123456789abcdef"),
			ExpiresAtUnix: now.Add(time.Hour).Unix(),
		}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, b.Start(ctx))
	_, err := os.Stat(path)
	require.NoError(t, err)

	conn, err := grpc.NewClient("unix://"+path, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	got, err := pbdataplane.NewLocalCredentialBrokerClient(conn).GetKey(ctx, &pbdataplane.GetKeyRequest{KeyId: "k43"})
	require.NoError(t, err)
	require.Equal(t, "k43", got.KeyId)
}

func TestStartRejectsTCPBind(t *testing.T) {
	b := New(Config{BindAddr: "127.0.0.1:0", Fetcher: staticFetcher{}})
	err := b.Start(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "unix socket")
}

func TestServeRejectsNonUnixScheme(t *testing.T) {
	b := New(Config{BindAddr: "tcp://127.0.0.1:9", Fetcher: staticFetcher{}})
	err := b.Serve(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "unix socket")
}

func TestStartEmptyBindIsNoop(t *testing.T) {
	b := New(Config{Fetcher: staticFetcher{}})
	require.NoError(t, b.Start(context.Background()))
}

type delayedFetcher struct {
	delay time.Duration
	resp  *pbdataplane.FetchKeyResponse
}

func (f delayedFetcher) FetchKey(context.Context, string) (*pbdataplane.FetchKeyResponse, error) {
	time.Sleep(f.delay)
	return f.resp, nil
}

func TestBrokerRejectsKeyThatExpiresDuringFetch(t *testing.T) {
	expiresAt := time.Now().Add(time.Second).Unix()
	b := New(Config{Fetcher: delayedFetcher{
		delay: 2 * time.Second,
		resp: &pbdataplane.FetchKeyResponse{
			KeyId:         "k43",
			KeyMaterial:   []byte("0123456789abcdef0123456789abcdef"),
			ExpiresAtUnix: expiresAt,
		},
	}})
	_, err := b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k43"})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Nil(t, b.get("k43"))
}

func TestGetKeyDropsExpiredCache(t *testing.T) {
	b := New(Config{Fetcher: staticFetcher{err: status.Error(codes.Unavailable, "down")}})
	b.put("k43", &cachedKey{
		resp:      &pbdataplane.FetchKeyResponse{KeyId: "k43", KeyMaterial: []byte("dek")},
		expiresAt: time.Now().Add(-time.Second),
	})
	_, err := b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k43"})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Nil(t, b.get("k43"))
}

func TestGetKeyKeepsCacheWhenRefreshErrors(t *testing.T) {
	now := time.Now()
	b := New(Config{Fetcher: staticFetcher{err: status.Error(codes.Unavailable, "down")}})
	b.put("k43", cacheFromFetchMust(t, &pbdataplane.FetchKeyResponse{
		KeyId:            "k43",
		KeyMaterial:      []byte("0123456789abcdef0123456789abcdef"),
		RefreshAfterUnix: now.Add(-time.Second).Unix(),
		ExpiresAtUnix:    now.Add(time.Hour).Unix(),
	}, 0.2))
	got, err := b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k43"})
	require.NoError(t, err)
	require.Equal(t, []byte("0123456789abcdef0123456789abcdef"), got.KeyMaterial)
}

func TestDoFetchSkipsWhenCacheIsFreshOrExpired(t *testing.T) {
	now := time.Now()
	b := New(Config{Fetcher: staticFetcher{err: status.Error(codes.Unavailable, "should not fetch")}})
	fresh := cacheFromFetchMust(t, &pbdataplane.FetchKeyResponse{
		KeyId:            "fresh",
		KeyMaterial:      []byte("0123456789abcdef0123456789abcdef"),
		RefreshAfterUnix: now.Add(time.Hour).Unix(),
		ExpiresAtUnix:    now.Add(2 * time.Hour).Unix(),
	}, 0.2)
	b.put("fresh", fresh)
	got, err := b.doFetch(context.Background(), "fresh")
	require.NoError(t, err)
	require.Equal(t, "fresh", got.KeyId)

	b.put("stale", &cachedKey{
		resp:      &pbdataplane.FetchKeyResponse{KeyId: "stale"},
		expiresAt: now.Add(-time.Second),
	})
	_, err = b.doFetch(context.Background(), "stale")
	require.ErrorIs(t, err, errKeyExpired)
	require.Nil(t, b.get("stale"))
}

func TestPutPrunesExpiredKeys(t *testing.T) {
	b := New(Config{})
	b.put("old", &cachedKey{expiresAt: time.Now().Add(-time.Second)})
	b.put("new", &cachedKey{expiresAt: time.Now().Add(time.Hour)})
	require.Nil(t, b.get("old"))
	require.NotNil(t, b.get("new"))
}

func TestToGetKeyNil(t *testing.T) {
	require.Empty(t, toGetKey(nil).KeyId)
}

func TestExitedBeforeStartIsNil(t *testing.T) {
	b := New(Config{})
	require.Nil(t, b.Exited())
}

func TestServeEmptyBind(t *testing.T) {
	b := New(Config{})
	require.NoError(t, b.Serve(context.Background()))
}

func TestServeNilFetcher(t *testing.T) {
	b := New(Config{BindAddr: "unix:///tmp/unused-cb-serve.sock"})
	err := b.Serve(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "Fetcher is required")
}

func TestListenFailsWhenPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "not-a-socket")
	require.NoError(t, os.Mkdir(sock, 0o755))
	// listenUnix removes the path before binding. A file inside the directory
	// keeps Remove from succeeding, so Listen fails on Linux and Darwin.
	require.NoError(t, os.WriteFile(filepath.Join(sock, "keep"), []byte("x"), 0o644))
	b := New(Config{BindAddr: sock, Fetcher: staticFetcher{}})
	err := b.Start(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "listen")

	err = b.Serve(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "listen")
}

func TestListenChmodError(t *testing.T) {
	orig := chmodSocket
	chmodSocket = func(string, os.FileMode) error { return errors.New("chmod failed") }
	t.Cleanup(func() { chmodSocket = orig })

	dir := t.TempDir()
	path := filepath.Join("/tmp", filepath.Base(dir)+"-cb-chmod.sock")
	t.Cleanup(func() { _ = os.Remove(path) })
	b := New(Config{BindAddr: path, Fetcher: staticFetcher{}})
	err := b.Start(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "chmod")
}

func TestServeReturnsBrokerExit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join("/tmp", filepath.Base(dir)+"-cb-serve-exit.sock")
	t.Cleanup(func() { _ = os.Remove(path) })
	b := New(Config{
		BindAddr: path,
		Fetcher: staticFetcher{resp: &pbdataplane.FetchKeyResponse{
			KeyId:         "k",
			KeyMaterial:   []byte("0123456789abcdef0123456789abcdef"),
			ExpiresAtUnix: time.Now().Add(time.Hour).Unix(),
		}},
		Logger: hclog.NewNullLogger(),
	})
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { errCh <- b.Serve(ctx) }()

	conn, err := grpc.NewClient("unix://"+path, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := pbdataplane.NewLocalCredentialBrokerClient(conn)
	require.Eventually(t, func() bool {
		_, err := client.GetKey(ctx, &pbdataplane.GetKeyRequest{KeyId: "k"})
		return err == nil
	}, 2*time.Second, 20*time.Millisecond)
	b.server.GracefulStop()
	select {
	case err := <-errCh:
		require.EqualError(t, err, "credential broker exited unexpectedly")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Serve to return")
	}
}

func TestFetchCoalescedCancelledWaiter(t *testing.T) {
	fetcher := &blockingFetcher{
		started: make(chan struct{}),
		release: make(chan struct{}),
		resp: &pbdataplane.FetchKeyResponse{
			KeyId:         "k43",
			KeyMaterial:   []byte("0123456789abcdef0123456789abcdef"),
			ExpiresAtUnix: time.Now().Add(time.Hour).Unix(),
		},
	}
	b := New(Config{Fetcher: fetcher})
	go func() {
		_, _ = b.GetKey(context.Background(), &pbdataplane.GetKeyRequest{KeyId: "k43"})
	}()
	select {
	case <-fetcher.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for FetchKey")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.fetchCoalesced(ctx, "k43")
	require.ErrorIs(t, err, context.Canceled)
	close(fetcher.release)
}

type failListener struct{}

func (failListener) Accept() (net.Conn, error) { return nil, errors.New("boom") }
func (failListener) Close() error              { return nil }
func (failListener) Addr() net.Addr            { return &net.UnixAddr{Net: "unix", Name: "fail"} }

func TestServeBackgroundAcceptError(t *testing.T) {
	b := New(Config{Logger: hclog.NewNullLogger()})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.serveBackground(ctx, failListener{}, "/tmp/unused-cb-fail.sock")
	select {
	case err := <-b.Exited():
		require.Error(t, err)
		require.Contains(t, err.Error(), "boom")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for broker exit")
	}
}

func TestBrokerStopSignalsUnexpectedExit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join("/tmp", filepath.Base(dir)+"-cb-stop.sock")
	t.Cleanup(func() { _ = os.Remove(path) })
	b := New(Config{
		BindAddr: path,
		Fetcher:  staticFetcher{resp: &pbdataplane.FetchKeyResponse{KeyId: "k"}},
		Logger:   hclog.NewNullLogger(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, b.Start(ctx))
	require.NotNil(t, b.Exited())
	conn, err := grpc.NewClient("unix://"+path, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := pbdataplane.NewLocalCredentialBrokerClient(conn)
	require.Eventually(t, func() bool {
		_, err := client.GetKey(ctx, &pbdataplane.GetKeyRequest{KeyId: "k"})
		return err == nil
	}, 2*time.Second, 20*time.Millisecond)
	b.server.GracefulStop()
	select {
	case err := <-b.Exited():
		require.EqualError(t, err, "credential broker exited unexpectedly")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for broker exit")
	}
}

func TestStartFailsWhenParentIsFile(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "not-a-dir")
	require.NoError(t, os.WriteFile(parent, []byte("x"), 0o644))
	b := New(Config{
		BindAddr: "unix://" + filepath.Join(parent, "cb.sock"),
		Fetcher:  staticFetcher{},
	})
	err := b.Start(context.Background())
	require.Error(t, err)
}
