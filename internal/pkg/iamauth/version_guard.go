package iamauth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// CommittedVersionReader reads the version committed to IAM's policy database,
// independently of the version loaded into IAM's authorization runtime.
type CommittedVersionReader interface {
	GetCommittedPolicyVersion(context.Context) (int64, error)
}

// VersionGuard keeps a bounded-age proof of IAM's committed policy version.
// A notification can raise the required version, but cannot renew the proof.
type VersionGuard struct {
	reader        CommittedVersionReader
	maxAge        time.Duration
	retryInterval time.Duration
	now           func() time.Time

	mu         sync.Mutex
	committed  int64
	observed   int64
	started    time.Time
	retryAfter time.Time
	reads      singleflight.Group
}

func NewVersionGuard(reader CommittedVersionReader, maxAge time.Duration) (*VersionGuard, error) {
	if reader == nil || maxAge <= 0 {
		return nil, fmt.Errorf("committed policy version reader and positive proof age are required")
	}
	retryInterval := maxAge / 2
	if retryInterval <= 0 {
		retryInterval = maxAge
	}
	return &VersionGuard{reader: reader, maxAge: maxAge, retryInterval: retryInterval, now: time.Now}, nil
}

// ObserveVersion raises the notification watermark without treating the
// notification as proof that IAM's persisted version was checked.
func (g *VersionGuard) ObserveVersion(version int64) {
	if g == nil || version <= 0 {
		return
	}
	g.mu.Lock()
	if version > g.observed {
		g.observed = version
	}
	g.mu.Unlock()
}

func (g *VersionGuard) proof() (int64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.committed, g.committed > 0 && g.committed >= g.observed &&
		!g.started.IsZero() && g.now().Before(g.started.Add(g.maxAge))
}

// Verify returns only a current proof. An unavailable IAM reader, a version
// regression, or a response that finished after its proof expired fails closed.
func (g *VersionGuard) Verify(ctx context.Context) (int64, error) {
	if g == nil {
		return 0, fmt.Errorf("committed policy version guard is unavailable")
	}
	if version, ok := g.proof(); ok {
		return version, nil
	}
	g.mu.Lock()
	cooldown := g.now().Before(g.retryAfter)
	g.mu.Unlock()
	if cooldown {
		return 0, fmt.Errorf("committed policy version proof unavailable during retry interval")
	}
	result := g.reads.DoChan("committed-version", func() (any, error) {
		if version, ok := g.proof(); ok {
			return version, nil
		}
		g.mu.Lock()
		cooldown := g.now().Before(g.retryAfter)
		g.mu.Unlock()
		if cooldown {
			return 0, fmt.Errorf("committed policy version proof unavailable during retry interval")
		}
		return g.read(ctx)
	})
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case read := <-result:
		if read.Err != nil {
			return 0, read.Err
		}
		if version, ok := g.proof(); ok {
			return version, nil
		}
		return 0, fmt.Errorf("committed policy version proof expired or is behind a notification")
	}
}

// Refresh reads IAM even while an earlier proof is valid, allowing a periodic
// caller to discover committed revocations before the hard age boundary.
func (g *VersionGuard) Refresh(ctx context.Context) error {
	if g == nil {
		return fmt.Errorf("committed policy version guard is unavailable")
	}
	_, err := g.read(ctx)
	return err
}

func (g *VersionGuard) read(ctx context.Context) (int64, error) {
	started := g.now() // Start of the remote read, not the response time.
	version, err := g.reader.GetCommittedPolicyVersion(ctx)
	if err != nil {
		g.scheduleRetry()
		return 0, fmt.Errorf("committed policy version read failed: %w", err)
	}
	if version <= 0 {
		g.scheduleRetry()
		return 0, fmt.Errorf("IAM returned an invalid committed policy version")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if version < g.committed || version < g.observed {
		g.retryAfter = g.now().Add(g.retryInterval)
		return 0, fmt.Errorf("IAM committed policy version is behind the observed watermark")
	}
	if !g.now().Before(started.Add(g.maxAge)) {
		g.retryAfter = g.now().Add(g.retryInterval)
		return 0, fmt.Errorf("committed policy version proof expired during read")
	}
	if version > g.committed || g.started.Before(started) {
		g.committed = version
		g.started = started
	}
	g.retryAfter = time.Time{}
	return g.committed, nil
}

func (g *VersionGuard) scheduleRetry() {
	g.mu.Lock()
	g.retryAfter = g.now().Add(g.retryInterval)
	g.mu.Unlock()
}

// Run polls IAM until ctx is cancelled. It does not turn a failed read into a
// renewed proof; request admission remains governed by Verify's hard boundary.
func (g *VersionGuard) Run(ctx context.Context, interval time.Duration) error {
	if g == nil || interval <= 0 || interval >= g.maxAge {
		return fmt.Errorf("version polling interval must be shorter than proof age")
	}
	_ = g.Refresh(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_ = g.Refresh(ctx)
		}
	}
}
