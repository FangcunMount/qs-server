// Package scheduler contains read-only Evaluation maintenance use cases.
package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type mismatchKind string

const (
	mismatchOutcomeWithoutEvaluatedStatus mismatchKind = "outcome_without_evaluated_status"
	mismatchLeaseRecoveryCandidate        mismatchKind = "lease_recovery_candidate"
	mismatchSuccessProjectionDrift        mismatchKind = "success_projection_drift"
	mismatchCanonicalOutcomeMissing       mismatchKind = "canonical_outcome_missing"
	mismatchRunStatusMismatch             mismatchKind = "run_status_mismatch"
	mismatchTerminalConflict              mismatchKind = "terminal_conflict"
	mismatchProjectionWithoutOutcome      mismatchKind = "projection_without_outcome"
	mismatchProjectionMissing             mismatchKind = "projection_missing"
	mismatchProjectionOutcomeMismatch     mismatchKind = "projection_outcome_mismatch"
	mismatchUnexpectedProjection          mismatchKind = "unexpected_projection"
	mismatchCommittedOutboxWithoutOutcome mismatchKind = "committed_outbox_without_outcome"
	mismatchCommittedOutboxMissing        mismatchKind = "committed_outbox_missing"
	mismatchCommittedOutboxMismatch       mismatchKind = "committed_outbox_reference_mismatch"
	mismatchRunOutcomeReferenceMismatch   mismatchKind = "run_outcome_reference_mismatch"
)

type mismatchSeverity string

const (
	severityHigh   mismatchSeverity = "high"
	severityMedium mismatchSeverity = "medium"
	severityLow    mismatchSeverity = "low"
)

type mismatch struct {
	AssessmentID      uint64
	Kind              mismatchKind
	Severity          mismatchSeverity
	RecommendedAction string
	DetectedAt        time.Time
}

type AuditBatchResult struct {
	Scanned       int
	Detected      int
	NextCursor    uint64
	CycleComplete bool
}

type Service interface {
	AuditBatch(context.Context, uint64, int) (AuditBatchResult, error)
}

type service struct {
	consistency evaluationconsistency.Reader
	now         func() time.Time
}

// NewService wires the complete read-only EV-R011 consistency matrix.
func NewService(consistency evaluationconsistency.Reader) Service {
	return &service{
		consistency: consistency,
		now:         time.Now,
	}
}

func (s *service) AuditBatch(ctx context.Context, afterID uint64, limit int) (AuditBatchResult, error) {
	if s == nil || s.consistency == nil {
		return AuditBatchResult{}, fmt.Errorf("evaluation consistency audit is not configured: batch evidence reader is required")
	}
	if limit <= 0 {
		return AuditBatchResult{CycleComplete: true}, nil
	}
	batch, err := s.consistency.ReadBatch(ctx, afterID, limit)
	if err != nil {
		return AuditBatchResult{}, err
	}
	detected := 0
	for _, evidence := range batch.Items {
		if evidence.AssessmentID == 0 {
			continue
		}
		items := classifyDrifts(consistencyEvidence{
			status:     domainassessment.Status(evidence.Status),
			outcome:    evidence.Outcome,
			run:        evidence.Run,
			projection: evidence.Projection,
			outbox:     evidence.Outbox,
		}, s.now())
		for _, item := range items {
			item.AssessmentID = evidence.AssessmentID
			observeMismatch(item.Kind)
			observeDisposition(item.Kind, "deferred")
			log.Warnf(
				"evaluation consistency drift requires audited migration (assessment_id=%d, kind=%s, severity=%s, action=%s)",
				item.AssessmentID, item.Kind, item.Severity, item.RecommendedAction,
			)
			detected++
		}
	}
	return AuditBatchResult{
		Scanned: len(batch.Items), Detected: detected, NextCursor: batch.NextCursor, CycleComplete: batch.CycleComplete,
	}, nil
}

type consistencyEvidence struct {
	status     domainassessment.Status
	outcome    *evaluationconsistency.OutcomeEvidence
	run        *evaluationconsistency.RunEvidence
	projection *evaluationconsistency.ProjectionEvidence
	outbox     *evaluationconsistency.CommittedOutboxEvidence
}

// classifyDrifts maps the complete Assessment/Run/Outcome/Projection/Outbox
// matrix to explicit read-only drift classes.
func classifyDrifts(evidence consistencyEvidence, now time.Time) []*mismatch {
	items := make([]*mismatch, 0, 4)
	add := func(kind mismatchKind, severity mismatchSeverity, action string) {
		items = append(items, &mismatch{
			Kind: kind, Severity: severity, RecommendedAction: action, DetectedAt: now,
		})
	}

	runStatus := evalrun.Status("")
	leaseExpired := false
	if evidence.run != nil {
		runStatus = evalrun.Status(evidence.run.Status)
		if runStatus == evalrun.StatusRunning {
			if lease := evidence.run.LeaseExpiresAt; lease != nil && !lease.After(now) {
				leaseExpired = true
			}
		}
	}

	if evidence.outcome == nil {
		if evidence.projection != nil && evidence.projection.RowCount > 0 {
			add(mismatchProjectionWithoutOutcome, severityHigh, "remove or rebuild projection only after locating the canonical outcome")
		}
		if evidence.outbox != nil && evidence.outbox.RowCount > 0 {
			add(mismatchCommittedOutboxWithoutOutcome, severityHigh, "quarantine committed event and investigate missing canonical outcome")
		}
	} else {
		outcomeID := evidence.outcome.ID
		if evidence.outcome.ModelKind == string(modelcatalog.KindScale) {
			switch {
			case evidence.projection == nil || evidence.projection.RowCount == 0:
				add(mismatchProjectionMissing, severityMedium, "rebuild scale projection from the canonical outcome in an audited maintenance window")
			case evidence.projection.UnlinkedRowCount > 0 ||
				evidence.projection.DistinctOutcomeCount != 1 ||
				evidence.projection.OutcomeID != outcomeID:
				add(mismatchProjectionOutcomeMismatch, severityHigh, "replace projection from the canonical outcome after operator confirmation")
			}
		} else if evidence.projection != nil && evidence.projection.RowCount > 0 {
			add(mismatchUnexpectedProjection, severityMedium, "inspect legacy scale projection attached to a non-scale outcome")
		}

		switch {
		case evidence.outbox == nil || evidence.outbox.RowCount == 0:
			add(mismatchCommittedOutboxMissing, severityHigh, "stage a governed replay only after verifying the committed outcome")
		case evidence.outbox.RowCount != 1 ||
			evidence.outbox.OutcomeID != outcomeID ||
			evidence.outbox.RunID != evidence.outcome.RunID:
			add(mismatchCommittedOutboxMismatch, severityHigh, "quarantine conflicting outbox evidence and require operator decision")
		}

		if evidence.run == nil || evidence.run.ID != evidence.outcome.RunID {
			add(mismatchRunOutcomeReferenceMismatch, severityHigh, "locate the exact run referenced by the canonical outcome")
		}
	}

	switch {
	case evidence.status.IsSubmitted() && evidence.outcome != nil && runStatus == evalrun.StatusSucceeded:
		add(mismatchSuccessProjectionDrift, severityMedium, "verify projection/outbox then migrate assessment to evaluated")
	case evidence.status.IsSubmitted() && evidence.outcome != nil:
		add(mismatchOutcomeWithoutEvaluatedStatus, severityHigh, "audited migration to evaluated after confirming canonical outcome")
	case evidence.status.IsSubmitted() && evidence.outcome == nil && leaseExpired:
		add(mismatchLeaseRecoveryCandidate, severityMedium, "lease recovery / redelivery; do not rewrite assessment status here")
	case evidence.status.IsEvaluated() && evidence.outcome == nil:
		add(mismatchCanonicalOutcomeMissing, severityHigh, "investigate missing outcome; never invent outcome from current catalog")
	case evidence.status.IsEvaluated() && evidence.outcome != nil && (runStatus == evalrun.StatusFailed || runStatus == evalrun.StatusRunning):
		add(mismatchRunStatusMismatch, severityMedium, "audit run/status mismatch; manual confirmation required")
	case evidence.status.IsFailed() && evidence.outcome != nil && runStatus == evalrun.StatusSucceeded:
		add(mismatchTerminalConflict, severityHigh, "terminal conflict; require operator decision")
	}
	return items
}

var (
	evaluationConsistencyMismatchTotal    = promauto.NewCounterVec(prometheus.CounterOpts{Namespace: "qs", Subsystem: "evaluation_consistency", Name: "mismatch_total", Help: "Total evaluation cross-store mismatches detected by the consistency audit."}, []string{"kind"})
	evaluationConsistencyDispositionTotal = promauto.NewCounterVec(prometheus.CounterOpts{Namespace: "qs", Subsystem: "evaluation_consistency", Name: "disposition_total", Help: "Total evaluation consistency mismatches by kind and audit disposition."}, []string{"kind", "disposition"})
)

func observeMismatch(kind mismatchKind) {
	evaluationConsistencyMismatchTotal.WithLabelValues(string(kind)).Inc()
}
func observeDisposition(kind mismatchKind, disposition string) {
	evaluationConsistencyDispositionTotal.WithLabelValues(string(kind), disposition).Inc()
}
