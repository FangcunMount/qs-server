package outboxready

import (
	"context"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

// Reconciler backfills Redis ready index from outbox facts.
type Reconciler struct {
	index    *Index
	lister   outboxport.PendingEventRefLister
	interval time.Duration
	limit    int
	mu       sync.Mutex
	started  bool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewReconciler(index *Index, lister outboxport.PendingEventRefLister, interval time.Duration) *Reconciler {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Reconciler{index: index, lister: lister, interval: interval, limit: 500}
}

func (r *Reconciler) Start(ctx context.Context) {
	if r == nil || r.index == nil || r.lister == nil {
		return
	}
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	ctx, r.cancel = context.WithCancel(ctx)
	r.started = true
	r.wg.Add(1)
	r.mu.Unlock()
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.runOnce(ctx)
			}
		}
	}()
}

func (r *Reconciler) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
}

func (r *Reconciler) runOnce(ctx context.Context) {
	now := time.Now()
	refs, err := r.lister.ListPendingEventRefs(ctx, r.limit, now)
	if err != nil {
		logger.L(ctx).Warnw("outbox ready index reconcile failed", "error", err.Error())
		return
	}
	for _, ref := range refs {
		if ref.EventID == "" {
			continue
		}
		nextAttemptAt := ref.NextAttemptAt
		if nextAttemptAt.IsZero() {
			nextAttemptAt = now
		}
		if err := r.index.Enqueue(ctx, ref.EventType, ref.EventID, nextAttemptAt, ref.CreatedAt); err != nil {
			logger.L(ctx).Warnw("outbox ready index enqueue failed",
				"event_id", ref.EventID,
				"event_type", ref.EventType,
				"error", err.Error(),
			)
		}
	}
}
