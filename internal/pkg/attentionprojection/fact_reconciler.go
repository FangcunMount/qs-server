package attentionprojection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

type FactReconcileResult struct {
	Scanned    int
	Missing    int
	Existing   int
	Mismatched int
	Created    int
	NextCursor string
}

// FactManifestGuard pins a recovery run to an immutable set of report facts.
// The fingerprint is computed from the full target facts, so a partially
// completed run can safely resume without widening its authorized scope.
type FactManifestGuard struct {
	ReportIDs   []string
	Fingerprint string
}

type FactReconciler struct {
	source      FactSource
	store       Store
	projector   *Projector
	runner      locklease.Runner
	from        time.Time
	dryRun      bool
	interval    time.Duration
	batchSize   int
	logger      *slog.Logger
	targets     map[string]struct{}
	fingerprint string

	mu                  sync.Mutex
	cursor              string
	started             bool
	consecutiveFailures int
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
}

func NewFactReconciler(source FactSource, store Store, projector *Projector, runner locklease.Runner, from time.Time, dryRun bool, interval time.Duration, batchSize int, guard FactManifestGuard, logger *slog.Logger) (*FactReconciler, error) {
	if source == nil || store == nil || projector == nil || runner == nil {
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
	targets, fingerprint, err := validateFactManifestGuard(guard)
	if err != nil {
		return nil, err
	}
	return &FactReconciler{
		source: source, store: store, projector: projector, runner: runner,
		from: from.UTC(), dryRun: dryRun, interval: interval, batchSize: batchSize,
		logger: logger, targets: targets, fingerprint: fingerprint,
	}, nil
}

// RunOnce executes one fact-recovery round only while this worker owns the
// shared Attention reconciliation leader lease. Contention is a normal skip.
func (r *FactReconciler) RunOnce(ctx context.Context) (result FactReconcileResult, err error) {
	if r == nil || r.runner == nil {
		return FactReconcileResult{}, fmt.Errorf("attention fact reconciler is not configured")
	}
	leaseResult, err := r.runner.Run(ctx, locklease.WorkloadAttentionProjectionReconcile, reconcileLeaseKey, 0, func(leaseCtx context.Context) error {
		result, err = r.runOnce(leaseCtx)
		return err
	})
	if err != nil {
		return result, err
	}
	if !leaseResult.Acquired {
		return FactReconcileResult{}, nil
	}
	return result, nil
}

func (r *FactReconciler) runOnce(ctx context.Context) (result FactReconcileResult, err error) {
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
	if len(r.targets) > 0 {
		result, err = r.runTargetedOnce(ctx)
		if err == nil {
			observeFactReconcile(result, r.dryRun)
		}
		return result, err
	}

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

func (r *FactReconciler) runTargetedOnce(ctx context.Context) (FactReconcileResult, error) {
	facts, err := r.loadTargetFacts(ctx)
	if err != nil {
		return FactReconcileResult{}, err
	}
	fingerprint, err := factManifestFingerprint(facts)
	if err != nil {
		return FactReconcileResult{}, err
	}
	if fingerprint != r.fingerprint {
		return FactReconcileResult{}, fmt.Errorf("attention fact manifest fingerprint mismatch: got %s want %s", fingerprint, r.fingerprint)
	}

	result := FactReconcileResult{Scanned: len(facts)}
	incomplete := make([]ReportFact, 0, len(facts))
	for _, fact := range facts {
		record, findErr := r.store.FindByReportID(ctx, fact.ReportID)
		switch {
		case findErr == nil:
			if record.AssessmentID != fact.AssessmentID || record.TesteeID != fact.TesteeID ||
				record.RiskLevel != fact.RiskLevel || record.MarkKeyFocus != fact.MarkKeyFocus {
				result.Mismatched++
				continue
			}
			if record.Status == StatusSucceeded {
				result.Existing++
				continue
			}
			result.Missing++
			incomplete = append(incomplete, fact)
		case errors.Is(findErr, ErrNotFound):
			result.Missing++
			incomplete = append(incomplete, fact)
		default:
			return result, findErr
		}
	}
	if result.Mismatched > 0 {
		return result, fmt.Errorf("attention fact manifest contains %d mismatched projections", result.Mismatched)
	}
	if r.dryRun {
		return result, nil
	}

	for _, fact := range incomplete {
		input := PendingInput{
			EventID:  "interpretation.report.generated.reconcile:" + fact.ReportID,
			ReportID: fact.ReportID, AssessmentID: fact.AssessmentID, TesteeID: fact.TesteeID,
			RiskLevel: fact.RiskLevel, MarkKeyFocus: fact.MarkKeyFocus,
		}
		if err := r.projector.Project(ctx, input); err != nil {
			return result, fmt.Errorf("project targeted attention report %s: %w", fact.ReportID, err)
		}
		record, err := r.store.FindByReportID(ctx, fact.ReportID)
		if err != nil {
			return result, fmt.Errorf("verify targeted attention report %s: %w", fact.ReportID, err)
		}
		if record.Status != StatusSucceeded {
			return result, fmt.Errorf("targeted attention report %s settled as %s", fact.ReportID, record.Status)
		}
		result.Created++
	}
	return result, nil
}

func (r *FactReconciler) loadTargetFacts(ctx context.Context) ([]ReportFact, error) {
	byReport := make(map[string]ReportFact, len(r.targets))
	cursor := ""
	for page := 0; page < 100; page++ {
		facts, next, err := r.source.ListReportFacts(ctx, r.from, cursor, r.batchSize)
		if err != nil {
			return nil, err
		}
		for _, fact := range facts {
			if _, wanted := r.targets[fact.ReportID]; !wanted {
				continue
			}
			if _, duplicate := byReport[fact.ReportID]; duplicate {
				return nil, fmt.Errorf("duplicate attention report fact %s", fact.ReportID)
			}
			byReport[fact.ReportID] = fact
		}
		if next == "" {
			break
		}
		if next == cursor {
			return nil, fmt.Errorf("attention fact source returned a non-advancing cursor")
		}
		cursor = next
		if page == 99 {
			return nil, fmt.Errorf("attention fact manifest scan exceeded 100 pages")
		}
	}
	if len(byReport) != len(r.targets) {
		return nil, fmt.Errorf("attention fact manifest resolved %d reports, want %d", len(byReport), len(r.targets))
	}
	facts := make([]ReportFact, 0, len(byReport))
	for _, fact := range byReport {
		facts = append(facts, fact)
	}
	return facts, nil
}

func validateFactManifestGuard(guard FactManifestGuard) (map[string]struct{}, string, error) {
	fingerprint := strings.TrimSpace(guard.Fingerprint)
	if len(guard.ReportIDs) == 0 && fingerprint == "" {
		return nil, "", nil
	}
	if len(guard.ReportIDs) == 0 || len(fingerprint) != sha256.Size*2 {
		return nil, "", fmt.Errorf("attention fact manifest report IDs and SHA-256 fingerprint are required together")
	}
	decoded, err := hex.DecodeString(fingerprint)
	if err != nil || len(decoded) != sha256.Size || fingerprint != strings.ToLower(fingerprint) {
		return nil, "", fmt.Errorf("attention fact manifest fingerprint must be lowercase SHA-256")
	}
	targets := make(map[string]struct{}, len(guard.ReportIDs))
	for _, reportID := range guard.ReportIDs {
		if _, err := strconv.ParseUint(reportID, 10, 64); err != nil || reportID == "0" {
			return nil, "", fmt.Errorf("invalid attention fact manifest report ID %q", reportID)
		}
		if _, exists := targets[reportID]; exists {
			return nil, "", fmt.Errorf("duplicate attention fact manifest report ID %s", reportID)
		}
		targets[reportID] = struct{}{}
	}
	return targets, fingerprint, nil
}

func factManifestFingerprint(facts []ReportFact) (string, error) {
	lines := make([]string, 0, len(facts))
	for _, fact := range facts {
		if _, err := strconv.ParseUint(fact.ReportID, 10, 64); err != nil || fact.ReportID == "0" {
			return "", fmt.Errorf("invalid attention report ID %q", fact.ReportID)
		}
		if _, err := strconv.ParseUint(fact.AssessmentID, 10, 64); err != nil || fact.AssessmentID == "0" {
			return "", fmt.Errorf("invalid attention assessment ID %q", fact.AssessmentID)
		}
		if fact.TesteeID == 0 || (fact.RiskLevel != "high" && fact.RiskLevel != "severe") || !fact.MarkKeyFocus {
			return "", fmt.Errorf("invalid attention fact for report %s", fact.ReportID)
		}
		lines = append(lines, strings.Join([]string{
			fact.ReportID, fact.AssessmentID, strconv.FormatUint(fact.TesteeID, 10),
			fact.RiskLevel, strconv.FormatBool(fact.MarkKeyFocus),
		}, "\t"))
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n") + "\n"))
	return hex.EncodeToString(sum[:]), nil
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
