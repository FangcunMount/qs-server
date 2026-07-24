package attentionprojection

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type FactReconcileResult struct {
	Scanned    int
	Missing    int
	Existing   int
	Mismatched int
	Created    int
	NextCursor string
}

type FactReconciler struct {
	source    FactSource
	store     Store
	projector *Projector
	from      time.Time
	dryRun    bool
	interval  time.Duration
	batchSize int
	logger    *slog.Logger

	mu                  sync.Mutex
	cursor              string
	started             bool
	consecutiveFailures int
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
}

func NewFactReconciler(source FactSource, store Store, projector *Projector, from time.Time, dryRun bool, interval time.Duration, batchSize int, logger *slog.Logger) (*FactReconciler, error) {
	if source == nil || store == nil || projector == nil {
		return nil, fmt.Errorf("attention fact reconciler dependencies are required")
	}
	if from.IsZero() {
		return nil, fmt.Errorf("attention projection reconcile_from is required")
	}
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	if batchSize <= 0 || batchSize > 500 {
		batchSize = 500
	}
	return &FactReconciler{source: source, store: store, projector: projector, from: from.UTC(), dryRun: dryRun, interval: interval, batchSize: batchSize, logger: logger}, nil
}

func (r *FactReconciler) RunOnce(ctx context.Context) (result FactReconcileResult, err error) {
	if r == nil {
		return FactReconcileResult{}, fmt.Errorf("attention fact reconciler is not configured")
	}
	startedAt := time.Now()
	dryRunLabel := fmt.Sprintf("%t", r.dryRun)
	defer func() {
		attentionFactReconcileDuration.WithLabelValues(dryRunLabel).Observe(time.Since(startedAt).Seconds())
		roundResult := "success"
		if err != nil {
			roundResult = "error"
		}
		attentionFactReconcileRounds.WithLabelValues(roundResult, dryRunLabel).Inc()
	}()
	r.mu.Lock()
	cursor := r.cursor
	r.mu.Unlock()

	facts, next, err := r.source.ListReportFacts(ctx, r.from, cursor, r.batchSize)
	if err != nil {
		return FactReconcileResult{}, err
	}
	result = FactReconcileResult{Scanned: len(facts), NextCursor: next}
	for _, fact := range facts {
		record, findErr := r.store.FindByReportID(ctx, fact.ReportID)
		switch {
		case findErr == nil:
			if record.AssessmentID != fact.AssessmentID || record.TesteeID != fact.TesteeID ||
				record.RiskLevel != fact.RiskLevel || record.MarkKeyFocus != fact.MarkKeyFocus {
				result.Mismatched++
			} else {
				result.Existing++
			}
		case errors.Is(findErr, ErrNotFound):
			result.Missing++
			if !r.dryRun {
				input := PendingInput{
					EventID:  "interpretation.report.generated.reconcile:" + fact.ReportID,
					ReportID: fact.ReportID, AssessmentID: fact.AssessmentID, TesteeID: fact.TesteeID,
					RiskLevel: fact.RiskLevel, MarkKeyFocus: fact.MarkKeyFocus,
				}
				if err := r.projector.Project(ctx, input); err != nil {
					return result, fmt.Errorf("project missing attention report %s: %w", fact.ReportID, err)
				}
				result.Created++
			}
		default:
			return result, findErr
		}
	}
	r.mu.Lock()
	r.cursor = next
	r.mu.Unlock()
	observeFactReconcile(result, r.dryRun)
	return result, nil
}

func (r *FactReconciler) Start(parent context.Context) {
	if r == nil {
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
				result, err := r.RunOnce(ctx)
				if err != nil {
					r.mu.Lock()
					r.consecutiveFailures++
					failures := r.consecutiveFailures
					r.mu.Unlock()
					attentionFactReconcileConsecutiveFailures.Set(float64(failures))
					if r.logger != nil {
						r.logger.Warn("attention fact reconcile failed", slog.String("error", err.Error()))
					}
				} else {
					r.mu.Lock()
					r.consecutiveFailures = 0
					r.mu.Unlock()
					attentionFactReconcileConsecutiveFailures.Set(0)
					if r.logger != nil && (result.Missing > 0 || result.Mismatched > 0) {
						r.logger.Warn("attention fact drift detected",
							slog.Int("missing", result.Missing), slog.Int("mismatched", result.Mismatched),
							slog.Bool("dry_run", r.dryRun),
						)
					}
				}
			}
		}
	}()
}

func (r *FactReconciler) Close() {
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
