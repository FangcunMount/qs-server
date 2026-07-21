package attentionprojection

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const defaultReconcileInterval = 30 * time.Second

// Reconciler retries pending/failed attention projections on a schedule.
type Reconciler struct {
	projector *Projector
	interval  time.Duration
	batchSize int
	logger    *slog.Logger

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewReconciler(projector *Projector, interval time.Duration, batchSize int, logger *slog.Logger) *Reconciler {
	if interval <= 0 {
		interval = defaultReconcileInterval
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	return &Reconciler{
		projector: projector,
		interval:  interval,
		batchSize: batchSize,
		logger:    logger,
	}
}

func (r *Reconciler) Start(parent context.Context) {
	if r == nil || r.projector == nil || r.projector.store == nil {
		return
	}
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
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
	records, err := r.projector.store.ListRetryable(ctx, r.projector.maxAttempts, r.batchSize)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("attention projection reconcile scan failed", slog.String("error", err.Error()))
		}
		return
	}
	for _, rec := range records {
		if err := r.projector.syncOnce(ctx, pendingInputFromRecord(rec)); err != nil && r.logger != nil {
			r.logger.Warn("attention projection reconcile retry failed",
				slog.String("event_id", rec.EventID),
				slog.String("error", err.Error()),
			)
		}
	}
}
