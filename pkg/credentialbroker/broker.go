// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package credentialbroker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hashicorp/consul-dataplane/internal/pbdataplane"
)

var errKeyExpired = errors.New("key expired")

// Fetcher loads a DEK from the Consul control plane.
type Fetcher interface {
	FetchKey(ctx context.Context, keyID string) (*pbdataplane.FetchKeyResponse, error)
}

// Config configures the local credential broker UDS server.
type Config struct {
	BindAddr        string
	RefreshFraction float64
	Fetcher         Fetcher
	Logger          hclog.Logger
}

// Broker is an in-memory Local Credential Broker. It never stores plaintext
// OAuth config — only DEKs keyed by key_id.
type Broker struct {
	pbdataplane.UnimplementedLocalCredentialBrokerServer
	cfg      Config
	mu       sync.RWMutex
	keys     map[string]*cachedKey
	server   *grpc.Server
	flightMu sync.Mutex
	flights  map[string]*fetchFlight
	exitedCh chan error
}

// fetchFlight coalesces concurrent FetchKey calls for the same key_id.
type fetchFlight struct {
	done chan struct{}
	resp *pbdataplane.GetKeyResponse
	err  error
}

type cachedKey struct {
	resp         *pbdataplane.FetchKeyResponse
	refreshAfter time.Time
	expiresAt    time.Time
}

// New returns a broker. BindAddr is unix:///path or a bare filesystem path.
func New(cfg Config) *Broker {
	if cfg.RefreshFraction <= 0 || cfg.RefreshFraction > 1 {
		cfg.RefreshFraction = 0.2
	}
	if cfg.Logger == nil {
		cfg.Logger = hclog.NewNullLogger()
	}
	return &Broker{
		cfg:     cfg,
		keys:    make(map[string]*cachedKey),
		flights: make(map[string]*fetchFlight),
	}
}

// Exited returns a channel that receives when the broker gRPC server stops
// unexpectedly. It is nil when the broker was not started (empty BindAddr).
func (b *Broker) Exited() <-chan error {
	return b.exitedCh
}

// Start binds the Unix socket (fail-closed) and serves GetKey in the background.
// An empty BindAddr is a no-op. TCP or other URL schemes are rejected.
func (b *Broker) Start(ctx context.Context) error {
	path, err := unixSocketPath(b.cfg.BindAddr)
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	if b.cfg.Fetcher == nil {
		return fmt.Errorf("credential broker: Fetcher is required")
	}
	lis, err := b.listenUnix(path)
	if err != nil {
		return err
	}
	b.serveBackground(ctx, lis, path)
	b.cfg.Logger.Info("credential broker listening", "path", path)
	return nil
}

func (b *Broker) Serve(ctx context.Context) error {
	path, err := unixSocketPath(b.cfg.BindAddr)
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	if b.cfg.Fetcher == nil {
		return fmt.Errorf("credential broker: Fetcher is required")
	}
	lis, err := b.listenUnix(path)
	if err != nil {
		return err
	}
	b.server = grpc.NewServer()
	pbdataplane.RegisterLocalCredentialBrokerServer(b.server, b)
	b.exitedCh = make(chan error, 1)
	go func() {
		<-ctx.Done()
		b.server.GracefulStop()
		_ = os.Remove(path)
	}()
	b.cfg.Logger.Info("credential broker listening", "path", path)
	err = b.server.Serve(lis)
	if err != nil && ctx.Err() == nil {
		b.exitedCh <- err
		return err
	}
	return nil
}

func (b *Broker) listenUnix(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("credential broker mkdir %q: %w", filepath.Dir(path), err)
	}
	_ = os.Remove(path)
	lis, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("credential broker listen %q: %w", path, err)
	}
	if err := os.Chmod(path, 0o660); err != nil {
		_ = lis.Close()
		return nil, fmt.Errorf("credential broker chmod %q: %w", path, err)
	}
	return wrapPeerCreds(lis, b.cfg.Logger), nil
}

func (b *Broker) serveBackground(ctx context.Context, lis net.Listener, path string) {
	b.server = grpc.NewServer()
	pbdataplane.RegisterLocalCredentialBrokerServer(b.server, b)
	b.exitedCh = make(chan error, 1)
	go func() {
		<-ctx.Done()
		b.server.GracefulStop()
		_ = os.Remove(path)
	}()
	go func() {
		err := b.server.Serve(lis)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			b.cfg.Logger.Error("credential broker serve failed", "error", err)
			b.exitedCh <- err
			return
		}
		b.exitedCh <- errors.New("credential broker exited unexpectedly")
	}()
}

func (b *Broker) GetKey(ctx context.Context, req *pbdataplane.GetKeyRequest) (*pbdataplane.GetKeyResponse, error) {
	keyID := req.GetKeyId()
	if keyID == "" {
		return nil, status.Error(codes.NotFound, "not found")
	}
	if b.cfg.Fetcher == nil {
		return nil, status.Error(codes.Internal, "broker misconfigured")
	}

	now := time.Now()
	if rec := b.get(keyID); rec != nil {
		if isExpired(rec, now) {
			b.drop(keyID)
			return nil, status.Error(codes.FailedPrecondition, "key expired")
		}
		if !needsRefresh(rec, now) {
			return toGetKey(rec.resp), nil
		}
	}

	resp, err := b.fetchCoalesced(ctx, keyID)
	if err != nil {
		if errors.Is(err, errKeyExpired) {
			if rec := b.get(keyID); rec != nil && !isExpired(rec, time.Now()) {
				return toGetKey(rec.resp), nil
			}
			return nil, status.Error(codes.FailedPrecondition, "key expired")
		}
		if rec := b.get(keyID); rec != nil && !isExpired(rec, time.Now()) {
			return toGetKey(rec.resp), nil
		}
		return nil, status.Error(codes.NotFound, "not found")
	}
	return resp, nil
}

func (b *Broker) fetchCoalesced(ctx context.Context, keyID string) (*pbdataplane.GetKeyResponse, error) {
	b.flightMu.Lock()
	if f, ok := b.flights[keyID]; ok {
		b.flightMu.Unlock()
		select {
		case <-f.done:
			return f.resp, f.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f := &fetchFlight{done: make(chan struct{})}
	b.flights[keyID] = f
	b.flightMu.Unlock()

	resp, err := b.doFetch(ctx, keyID)
	f.resp, f.err = resp, err
	b.flightMu.Lock()
	delete(b.flights, keyID)
	b.flightMu.Unlock()
	close(f.done)
	return resp, err
}

func (b *Broker) doFetch(ctx context.Context, keyID string) (*pbdataplane.GetKeyResponse, error) {
	now := time.Now()
	if rec := b.get(keyID); rec != nil {
		if isExpired(rec, now) {
			b.drop(keyID)
			return nil, errKeyExpired
		}
		if !needsRefresh(rec, now) {
			return toGetKey(rec.resp), nil
		}
	}
	fetched, err := b.cfg.Fetcher.FetchKey(ctx, keyID)
	if err != nil {
		return nil, err
	}
	rec, err := cacheFromFetch(fetched, b.cfg.RefreshFraction)
	if err != nil {
		return nil, err
	}
	if isExpired(rec, now) {
		return nil, errKeyExpired
	}
	b.put(keyID, rec)
	return toGetKey(rec.resp), nil
}

func (b *Broker) GetBrokerStatus(context.Context, *pbdataplane.GetBrokerStatusRequest) (*pbdataplane.GetBrokerStatusResponse, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return &pbdataplane.GetBrokerStatusResponse{
		Ready:      true,
		KeysCached: int32(len(b.keys)),
	}, nil
}

func (b *Broker) get(keyID string) *cachedKey {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.keys[keyID]
}

func (b *Broker) put(keyID string, rec *cachedKey) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	for id, k := range b.keys {
		if isExpired(k, now) {
			delete(b.keys, id)
		}
	}
	b.keys[keyID] = rec
}

func (b *Broker) drop(keyID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.keys, keyID)
}

func isExpired(rec *cachedKey, now time.Time) bool {
	return !rec.expiresAt.IsZero() && !now.Before(rec.expiresAt)
}

func needsRefresh(rec *cachedKey, now time.Time) bool {
	return !rec.refreshAfter.IsZero() && !now.Before(rec.refreshAfter)
}

func cacheFromFetch(resp *pbdataplane.FetchKeyResponse, fraction float64) (*cachedKey, error) {
	if resp == nil {
		return nil, fmt.Errorf("nil FetchKeyResponse")
	}
	rec := &cachedKey{resp: resp}
	if resp.ExpiresAtUnix > 0 {
		rec.expiresAt = time.Unix(resp.ExpiresAtUnix, 0)
	}
	if resp.RefreshAfterUnix > 0 {
		rec.refreshAfter = time.Unix(resp.RefreshAfterUnix, 0)
	} else if resp.NotBeforeUnix > 0 && resp.ExpiresAtUnix > resp.NotBeforeUnix {
		lifetime := time.Unix(resp.ExpiresAtUnix, 0).Sub(time.Unix(resp.NotBeforeUnix, 0))
		rec.refreshAfter = rec.expiresAt.Add(-time.Duration(float64(lifetime) * fraction))
	}
	return rec, nil
}

func toGetKey(resp *pbdataplane.FetchKeyResponse) *pbdataplane.GetKeyResponse {
	if resp == nil {
		return &pbdataplane.GetKeyResponse{}
	}
	return &pbdataplane.GetKeyResponse{
		KeyId:            resp.KeyId,
		KeyMaterial:      append([]byte(nil), resp.KeyMaterial...),
		Algorithm:        resp.Algorithm,
		NotBeforeUnix:    resp.NotBeforeUnix,
		RefreshAfterUnix: resp.RefreshAfterUnix,
		ExpiresAtUnix:    resp.ExpiresAtUnix,
	}
}

func unixSocketPath(bind string) (string, error) {
	bind = strings.TrimSpace(bind)
	if bind == "" {
		return "", nil
	}
	if strings.HasPrefix(bind, "unix://") {
		path := strings.TrimPrefix(bind, "unix://")
		if path == "" {
			return "", fmt.Errorf("credential broker bind address unix socket path is empty")
		}
		return path, nil
	}
	if strings.Contains(bind, "://") {
		return "", fmt.Errorf("credential broker bind address must be a unix socket")
	}
	if _, _, err := net.SplitHostPort(bind); err == nil {
		return "", fmt.Errorf("credential broker bind address must be a unix socket")
	}
	return bind, nil
}
