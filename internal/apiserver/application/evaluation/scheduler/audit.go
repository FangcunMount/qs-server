// Package scheduler contains read-only Evaluation maintenance use cases.
package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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

type Service interface {
	AuditOnce(context.Context, int) (int, error)
}

// SubmittedCandidateReader is the scheduler-specific keyset scan port. It has
// no user-facing pagination semantics.
type SubmittedCandidateReader interface {
	ListSubmittedAssessmentIDsAfter(context.Context, uint64, int) ([]uint64, error)
}

// LatestRunReader is optional; when nil, AuditOnce classifies without Run evidence.
type LatestRunReader interface {
	FindLatestByAssessmentID(ctx context.Context, assessmentID uint64) (*evalrun.EvaluationRun, error)
}

type service struct {
	assessments domainassessment.Repository
	outcomes    domainoutcome.Repository
	runs        LatestRunReader
	consistency evaluationconsistency.Reader
	reader      SubmittedCandidateReader
	mu          sync.Mutex
	cursor      uint64
	now         func() time.Time
}

// NewService wires the complete read-only EV-R011 consistency matrix.
func NewService(
	assessments domainassessment.Repository,
	outcomes domainoutcome.Repository,
	reader SubmittedCandidateReader,
	runs LatestRunReader,
	consistency evaluationconsistency.Reader,
) Service {
	return &service{
		assessments: assessments,
		outcomes:    outcomes,
		runs:        runs,
		consistency: consistency,
		reader:      reader,
		now:         time.Now,
	}
}

func (s *service) AuditOnce(ctx context.Context, limit int) (int, error) {
	if s == nil || s.assessments == nil || s.outcomes == nil || s.reader == nil || s.runs == nil || s.consistency == nil {
		return 0, fmt.Errorf("evaluation consistency audit is not configured: assessment, run, outcome, projection, outbox and candidate readers are required")
	}
	if limit <= 0 {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ids, err := s.reader.ListSubmittedAssessmentIDsAfter(ctx, s.cursor, limit)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		s.cursor = 0
		return 0, nil
	}
	detected := 0
	for _, assessmentID := range ids {
		if assessmentID == 0 {
			continue
		}
		items, scanErr := s.scanOne(ctx, assessmentID)
		if scanErr != nil {
			return 0, scanErr
		}
		for _, item := range items {
			observeMismatch(item.Kind)
			observeDisposition(item.Kind, "deferred")
			log.Warnf(
				"evaluation consistency drift requires audited migration (assessment_id=%d, kind=%s, severity=%s, action=%s)",
				item.AssessmentID, item.Kind, item.Severity, item.RecommendedAction,
			)
			detected++
		}
	}
	s.cursor = ids[len(ids)-1]
	return detected, nil
}

func (s *service) scanOne(ctx context.Context, assessmentID uint64) ([]*mismatch, error) {
	a, err := s.assessments.FindByID(ctx, domainassessment.NewID(assessmentID))
	if err != nil || a == nil {
		return nil, err
	}
	record, err := s.outcomes.FindByAssessmentID(ctx, meta.FromUint64(assessmentID))
	if err != nil {
		return nil, err
	}
	run, err := s.runs.FindLatestByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	projection, err := s.consistency.FindProjectionEvidence(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	outbox, err := s.consistency.FindCommittedOutboxEvidence(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	items := classifyDrifts(consistencyEvidence{
		status:     a.Status(),
		outcome:    record,
		run:        run,
		projection: projection,
		outbox:     outbox,
	}, s.now())
	for _, item := range items {
		item.AssessmentID = assessmentID
	}
	return items, nil
}

type consistencyEvidence struct {
	status     domainassessment.Status
	outcome    *domainoutcome.Record
	run        *evalrun.EvaluationRun
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
		runStatus = evidence.run.Attempt().Status
		if runStatus == evalrun.StatusRunning {
			if lease := evidence.run.LeaseExpiresAt(); lease != nil && !lease.After(now) {
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
		outcomeID := evidence.outcome.ID().String()
		if evidence.outcome.Model().Kind == modelcatalog.KindScale {
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
			evidence.outbox.RunID != evidence.outcome.RunID():
			add(mismatchCommittedOutboxMismatch, severityHigh, "quarantine conflicting outbox evidence and require operator decision")
		}

		if evidence.run == nil || evidence.run.ID().String() != evidence.outcome.RunID() {
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

// Ensure evaluationrun.Repository satisfies LatestRunReader when wired.
var _ LatestRunReader = evaluationrun.Repository(nil)
