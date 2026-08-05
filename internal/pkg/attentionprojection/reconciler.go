package attentionprojection

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

const defaultReconcileInterval = 30 * time.Second
const reconcileLeaseKey = "global"

// Reconciler retries pending/failed attention projections on a schedule.
type Reconciler struct {
	projector *Projector
	runner    locklease.Runner
	interval  time.Duration
	batchSize int
	logger    *slog.Logger

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewReconciler(projector *Projector, runner locklease.Runner, interval time.Duration, batchSize int, logger *slog.Logger) (*Reconciler, error) {
	if projector == nil || runner == nil {
		return nil, fmt.Errorf("attention projection reconciler dependencies are required")
	}
	if interval <= 0 {
		interval = defaultReconcileInterval
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	return &Reconciler{
		projector: projector,
		runner:    runner,
		interval:  interval,
		batchSize: batchSize,
		logger:    logger,
	}, nil
}

func (r *Reconciler) Start(parent context.Context) {
	if r == nil || r.projector == nil || r.projector.store == nil || r.runner == nil {
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
				if _, err := r.RunOnce(ctx); err != nil && r.logger != nil {
					r.logger.Warn("attention projection reconcile lease round failed", slog.String("error", err.Error()))
				}
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

// RunOnce executes one retry round only while this worker owns the shared
// Attention reconciliation leader lease. Contention is a normal skipped round.
func (r *Reconciler) RunOnce(ctx context.Context) (bool, error) {
	if r == nil || r.runner == nil {
		return false, fmt.Errorf("attention projection reconciler is not configured")
	}
	result, err := r.runner.Run(ctx, locklease.WorkloadAttentionProjectionReconcile, reconcileLeaseKey, 0, r.runOnce)
	return result.Acquired, err
}

func (r *Reconciler) runOnce(ctx context.Context) error {
	records, err := r.projector.store.ListRetryable(ctx, r.projector.maxAttempts, r.batchSize)
	if err != nil {
		return fmt.Errorf("scan retryable attention projections: %w", err)
	}
	for _, rec := range records {
		if err := r.projector.syncOnce(ctx, pendingInputFromRecord(rec)); err != nil && r.logger != nil {
			r.logger.Warn("attention projection reconcile retry failed",
				slog.String("event_id", rec.EventID),
				slog.String("error", err.Error()),
			)
		}
	}
	return nil
}
