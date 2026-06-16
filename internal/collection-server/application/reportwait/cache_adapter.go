package reportwait

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

// cacheAdapter 适配 reportstatus.Cache 到 reportwait.StatusCache。
type cacheAdapter struct {
	inner *reportstatus.Cache
}

func NewStatusCache(inner *reportstatus.Cache) StatusCache {
	if inner == nil {
		return nil
	}
	return &cacheAdapter{inner: inner}
}

func (a *cacheAdapter) Get(ctx context.Context, assessmentID string) (*reportstatus.Snapshot, error) {
	return a.inner.Get(ctx, assessmentID)
}

func (a *cacheAdapter) Set(ctx context.Context, snapshot *reportstatus.Snapshot, ttl time.Duration) error {
	return a.inner.Set(ctx, snapshot, ttl)
}

func (a *cacheAdapter) SetIfHigherPriority(ctx context.Context, snapshot *reportstatus.Snapshot, ttl time.Duration) error {
	return a.inner.SetIfHigherPriority(ctx, snapshot, ttl)
}

var ErrCacheUnavailable = reportstatus.ErrCacheUnavailable
