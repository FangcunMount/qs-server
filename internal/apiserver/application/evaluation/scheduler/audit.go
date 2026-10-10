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
	eventevidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
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
	mismatchHistoricalEventGap            mismatchKind = "historical_event_unverifiable"
	mismatchReverseOutboxConflict         mismatchKind = "standard_outbox_reverse_conflict"
	mismatchHistoricalReferenceConflict   mismatchKind = "historical_reference_conflict"
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
	Scanned  int
	Detected int
	// HistoricalGaps are explicit retained unverifiable conclusions, not
	// current standard-message verification successes. Detected keeps its
	// existing meaning; callers can separate these from actionable drifts.
	HistoricalGaps int
	NextCursor     uint64
	CycleComplete  bool
}

type Service interface {
	AuditBatch(context.Context, uint64, int) (AuditBatchResult, error)
}

// CycleService guarantees bounded forward and reverse scans as one audit cycle.
type CycleService interface {
	BusinessUpperBound(context.Context) (uint64, error)
	AuditBatchTo(context.Context, uint64, uint64, int) (AuditBatchResult, error)
	OutboxUpperBound(context.Context) (uint64, error)
	AuditOutboxBatch(context.Context, uint64, uint64, int) (AuditBatchResult, error)
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
		return AuditBatchResult{}, fmt.Errorf("evaluation consistency audit requires a reader")
	}
	if limit <= 0 {
		return AuditBatchResult{CycleComplete: true}, nil
	}
	batch, err := s.consistency.ReadBatch(ctx, afterID, limit)
	if err != nil {
		return AuditBatchResult{}, err
	}
	return s.classifyBatch(batch), nil
}
func (s *service) BusinessUpperBound(ctx context.Context) (uint64, error) {
	reader, err := s.cycleReader()
	if err != nil {
		return 0, err
	}
	return reader.BusinessUpperBound(ctx)
}
func (s *service) OutboxUpperBound(ctx context.Context) (uint64, error) {
	reader, err := s.cycleReader()
	if err != nil {
		return 0, err
	}
	return reader.OutboxUpperBound(ctx)
}
func (s *service) cycleReader() (evaluationconsistency.CycleReader, error) {
	if s == nil {
		return nil, fmt.Errorf("evaluation consistency audit requires a reader")
	}
	reader, ok := s.consistency.(evaluationconsistency.CycleReader)
	if !ok {
		return nil, fmt.Errorf("evaluation consistency audit requires bounded bidirectional reader")
	}
	return reader, nil
}
func (s *service) AuditBatchTo(ctx context.Context, after, upper uint64, limit int) (AuditBatchResult, error) {
	reader, err := s.cycleReader()
	if err != nil {
		return AuditBatchResult{}, err
	}
	batch, err := reader.ReadBatchTo(ctx, after, upper, limit)
	if err != nil {
		return AuditBatchResult{}, err
	}
	return s.classifyBatch(batch), nil
}
func (s *service) AuditOutboxBatch(ctx context.Context, after, upper uint64, limit int) (AuditBatchResult, error) {
	reader, err := s.cycleReader()
	if err != nil {
		return AuditBatchResult{}, err
	}
	batch, err := reader.ReadOutboxBatch(ctx, after, upper, limit)
	if err != nil {
		return AuditBatchResult{}, err
	}
	for _, conflict := range batch.Conflicts {
		observeMismatch(mismatchReverseOutboxConflict)
		observeDisposition(mismatchReverseOutboxConflict, "deferred")
		log.Warnf("evaluation reverse consistency conflict (message_id=%s, assessment_id=%d, reason=%s)", conflict.MessageID, conflict.AssessmentID, conflict.Reason)
	}
	return AuditBatchResult{Scanned: batch.Scanned, Detected: len(batch.Conflicts), NextCursor: batch.NextCursor, CycleComplete: batch.CycleComplete}, nil
}
func (s *service) classifyBatch(batch evaluationconsistency.Batch) AuditBatchResult {
	return s.classifyReadOnlyBatch(batch, true)
}

// SummarizeReadOnlyBatch reuses the scheduler's complete consistency matrix
// without logging business/message identities or updating runtime metrics.
// It does not read/write a checkpoint or grant repair/retirement authority.
func SummarizeReadOnlyBatch(batch evaluationconsistency.Batch, now time.Time) AuditBatchResult {
	return (&service{now: func() time.Time { return now }}).classifyReadOnlyBatch(batch, false)
}

func (s *service) classifyReadOnlyBatch(batch evaluationconsistency.Batch, observe bool) AuditBatchResult {
	detected := 0
	historicalGaps := 0
	for _, evidence := range batch.Items {
		if evidence.AssessmentID == 0 {
			continue
		}
		if observe && evidence.Outbox != nil && evidence.Outbox.Class != "" {
			evaluationConsistencyEvidenceClasses.WithLabelValues(string(evidence.Outbox.Class)).Inc()
		}
		if history := evidence.CommittedHistory; observe && history != nil && (history.Class == eventevidence.RetiredVerified || history.Class == eventevidence.Unverifiable) {
			evaluationCommittedHistoricalClasses.WithLabelValues(string(history.Class)).Inc()
		}
		items := classifyDrifts(consistencyEvidence{
			status:           domainassessment.Status(evidence.Status),
			outcome:          evidence.Outcome,
			run:              evidence.Run,
			projection:       evidence.Projection,
			outbox:           evidence.Outbox,
			committedHistory: evidence.CommittedHistory,
		}, s.now())
		for _, reference := range evidence.HistoricalReferences {
			if reference.InvalidReason != "" {
				items = append(items, &mismatch{Kind: mismatchHistoricalReferenceConflict, Severity: severityHigh, RecommendedAction: "investigate retained historical owner or source conflict; never manufacture or resend the original message", DetectedAt: s.now()})
				continue
			}
			switch reference.Class {
			case eventevidence.RetiredVerified:
				if observe {
					evaluationHistoricalReferenceClasses.WithLabelValues(string(reference.Class)).Inc()
				}
			case eventevidence.Unverifiable:
				if observe {
					evaluationHistoricalReferenceClasses.WithLabelValues(string(reference.Class)).Inc()
				}
				items = append(items, &mismatch{Kind: mismatchHistoricalEventGap, Severity: severityLow, RecommendedAction: "retain the explicit terminal historical gap; full original message verification remains unavailable", DetectedAt: s.now()})
			default:
				items = append(items, &mismatch{Kind: mismatchHistoricalReferenceConflict, Severity: severityHigh, RecommendedAction: "investigate invalid historical provenance classification", DetectedAt: s.now()})
			}
		}
		for _, item := range items {
			if item.Kind == mismatchHistoricalEventGap && item.Severity == severityLow {
				historicalGaps++
			}
			item.AssessmentID = evidence.AssessmentID
			if observe {
				observeMismatch(item.Kind)
				observeDisposition(item.Kind, "deferred")
				log.Warnf(
					"evaluation consistency drift requires audited migration (assessment_id=%d, kind=%s, severity=%s, action=%s)",
					item.AssessmentID, item.Kind, item.Severity, item.RecommendedAction,
				)
			}
			detected++
		}
	}
	return AuditBatchResult{
		Scanned: len(batch.Items), Detected: detected, HistoricalGaps: historicalGaps, NextCursor: batch.NextCursor, CycleComplete: batch.CycleComplete,
	}
}

type consistencyEvidence struct {
	status           domainassessment.Status
	outcome          *evaluationconsistency.OutcomeEvidence
	run              *evaluationconsistency.RunEvidence
	projection       *evaluationconsistency.ProjectionEvidence
	outbox           *evaluationconsistency.CommittedOutboxEvidence
	committedHistory *evaluationconsistency.CommittedHistoricalEvidence
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
		case evidence.outbox != nil && evidence.outbox.LegacyCanonicalAbsent && evidence.outbox.Class == "" && evidence.outbox.RowCount == 0 && evidence.committedHistory != nil && evidence.committedHistory.OutcomeID == outcomeID && evidence.committedHistory.RunID == evidence.outcome.RunID && (evidence.committedHistory.Class == eventevidence.RetiredVerified || evidence.committedHistory.Class == eventevidence.Unverifiable):
			// The physical canonical pair is empty and the old owner/run has
			// qualified retained provenance. Gap findings remain per original ID
			// below; do not double-count them or claim an SDK message match.
		case evidence.outbox != nil && evidence.outbox.InvalidReason != "":
			add(mismatchCommittedOutboxMismatch, severityHigh, "investigate classified evidence conflict; never synthesize or resend an event to erase uncertainty")
		case evidence.outbox != nil && evidence.outbox.Class == eventevidence.Unverifiable:
			add(mismatchHistoricalEventGap, severityLow, "retain the terminal historical gap; original message verification is unavailable")
		case evidence.outbox != nil && evidence.outbox.Class == eventevidence.RetiredVerified:
			// Verified retirement is recorded as historical provenance, not a live message match.
		case evidence.outbox == nil || evidence.outbox.RowCount == 0:
			add(mismatchCommittedOutboxMissing, severityHigh, "investigate missing canonical event evidence; preserve unknown execution state")
		case evidence.outbox.RowCount != 1 ||
			evidence.outbox.OutcomeID != outcomeID ||
			evidence.outbox.RunID != evidence.outcome.RunID:
			add(mismatchCommittedOutboxMismatch, severityHigh, "quarantine conflicting outbox evidence and require operator decision")
		}

		// The reader already rechecked physical absence of the canonical
		// original ID and its stable owner. Another retained, closed Run is
		// not that missing original. Never exempt an unfinished/unknown Run
		// or any retained lease; their existing matrix findings remain.
		gap := evidence.status.IsEvaluated() && evidence.outbox != nil && evidence.outbox.LegacyCanonicalAbsent && evidence.outbox.Class == "" && evidence.outbox.RowCount == 0 && evidence.outbox.InvalidReason == "canonical outcome lacks classified committed event evidence" && evidence.committedHistory != nil && evidence.committedHistory.Class == eventevidence.Unverifiable && evidence.committedHistory.OutcomeID == outcomeID && evidence.committedHistory.RunID == evidence.outcome.RunID
		if gap {
			gap = false
			for _, reason := range evidence.committedHistory.Reasons {
				if reason == "original_outcome_run_absent" {
					gap = true
				}
			}
		}
		if evidence.run != nil && (runStatus != evalrun.StatusSucceeded && runStatus != evalrun.StatusFailed || evidence.run.LeaseExpiresAt != nil) {
			gap = false
		}
		if (evidence.run == nil || evidence.run.ID != evidence.outcome.RunID) && !gap {
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
	evaluationCommittedHistoricalClasses  = promauto.NewCounterVec(prometheus.CounterOpts{Namespace: "qs", Subsystem: "evaluation_consistency", Name: "committed_historical_classes_total", Help: "Qualified historical-only committed owners with physically empty canonical pairs; no standard message fingerprint claim."}, []string{"class"})
	evaluationHistoricalReferenceClasses  = promauto.NewCounterVec(prometheus.CounterOpts{Namespace: "qs", Subsystem: "evaluation_consistency", Name: "historical_reference_classes_total", Help: "Business-local retained historical conclusions, independently classified; not standard message verification or full legacy source coverage."}, []string{"class"})
	evaluationConsistencyEvidenceClasses  = promauto.NewCounterVec(prometheus.CounterOpts{Namespace: "qs", Subsystem: "evaluation_consistency", Name: "event_evidence_classes_total", Help: "Classified event provenance; historical receipts do not claim current message verification."}, []string{"class"})
	evaluationConsistencyMismatchTotal    = promauto.NewCounterVec(prometheus.CounterOpts{Namespace: "qs", Subsystem: "evaluation_consistency", Name: "mismatch_total", Help: "Total evaluation cross-store mismatches detected by the consistency audit."}, []string{"kind"})
	evaluationConsistencyDispositionTotal = promauto.NewCounterVec(prometheus.CounterOpts{Namespace: "qs", Subsystem: "evaluation_consistency", Name: "disposition_total", Help: "Total evaluation consistency mismatches by kind and audit disposition."}, []string{"kind", "disposition"})
)

func observeMismatch(kind mismatchKind) {
	evaluationConsistencyMismatchTotal.WithLabelValues(string(kind)).Inc()
}
func observeDisposition(kind mismatchKind, disposition string) {
	evaluationConsistencyDispositionTotal.WithLabelValues(string(kind), disposition).Inc()
}
