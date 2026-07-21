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
	reader      SubmittedCandidateReader
	mu          sync.Mutex
	cursor      uint64
	now         func() time.Time
}

func NewService(assessments domainassessment.Repository, outcomes domainoutcome.Repository, reader SubmittedCandidateReader) Service {
	return NewServiceWithRuns(assessments, outcomes, reader, nil)
}

// NewServiceWithRuns wires optional Run lookup for EV-R011 matrix classification.
func NewServiceWithRuns(
	assessments domainassessment.Repository,
	outcomes domainoutcome.Repository,
	reader SubmittedCandidateReader,
	runs LatestRunReader,
) Service {
	return &service{
		assessments: assessments,
		outcomes:    outcomes,
		runs:        runs,
		reader:      reader,
		now:         time.Now,
	}
}

func (s *service) AuditOnce(ctx context.Context, limit int) (int, error) {
	if s == nil || s.assessments == nil || s.outcomes == nil || s.reader == nil {
		return 0, fmt.Errorf("evaluation consistency audit is not configured: assessment, outcome and candidate repositories are required")
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
		item, scanErr := s.scanOne(ctx, assessmentID)
		if scanErr != nil {
			return 0, scanErr
		}
		if item == nil {
			continue
		}
		observeMismatch(item.Kind)
		observeDisposition(item.Kind, "deferred")
		log.Warnf(
			"evaluation consistency drift requires audited migration (assessment_id=%d, kind=%s, severity=%s, action=%s)",
			item.AssessmentID, item.Kind, item.Severity, item.RecommendedAction,
		)
		detected++
	}
	s.cursor = ids[len(ids)-1]
	return detected, nil
}

func (s *service) scanOne(ctx context.Context, assessmentID uint64) (*mismatch, error) {
	a, err := s.assessments.FindByID(ctx, domainassessment.NewID(assessmentID))
	if err != nil || a == nil {
		return nil, err
	}
	record, err := s.outcomes.FindByAssessmentID(ctx, meta.FromUint64(assessmentID))
	if err != nil {
		return nil, err
	}
	var run *evalrun.EvaluationRun
	if s.runs != nil {
		run, err = s.runs.FindLatestByAssessmentID(ctx, assessmentID)
		if err != nil {
			return nil, err
		}
	}
	item := classifyDrift(a.Status(), record != nil, run, s.now())
	if item == nil {
		return nil, nil
	}
	item.AssessmentID = assessmentID
	return item, nil
}

// classifyDrift maps Assessment/Run/Outcome evidence to EV-R011 drift classes.
// Projection/Outbox columns remain deferred until dedicated readers exist; this
// function stays read-only and never mutates aggregates.
func classifyDrift(
	status domainassessment.Status,
	hasOutcome bool,
	run *evalrun.EvaluationRun,
	now time.Time,
) *mismatch {
	runStatus := evalrun.Status("")
	leaseExpired := false
	if run != nil {
		runStatus = run.Attempt().Status
		if runStatus == evalrun.StatusRunning {
			if lease := run.LeaseExpiresAt(); lease != nil && !lease.After(now) {
				leaseExpired = true
			}
		}
	}

	switch {
	case status.IsSubmitted() && hasOutcome && runStatus == evalrun.StatusSucceeded:
		return &mismatch{
			Kind: mismatchSuccessProjectionDrift, Severity: severityMedium,
			RecommendedAction: "verify projection/outbox then migrate assessment to evaluated",
			DetectedAt:        now,
		}
	case status.IsSubmitted() && hasOutcome:
		return &mismatch{
			Kind: mismatchOutcomeWithoutEvaluatedStatus, Severity: severityHigh,
			RecommendedAction: "audited migration to evaluated after confirming canonical outcome",
			DetectedAt:        now,
		}
	case status.IsSubmitted() && !hasOutcome && leaseExpired:
		return &mismatch{
			Kind: mismatchLeaseRecoveryCandidate, Severity: severityMedium,
			RecommendedAction: "lease recovery / redelivery; do not rewrite assessment status here",
			DetectedAt:        now,
		}
	case status.IsEvaluated() && !hasOutcome:
		return &mismatch{
			Kind: mismatchCanonicalOutcomeMissing, Severity: severityHigh,
			RecommendedAction: "investigate missing outcome; never invent outcome from current catalog",
			DetectedAt:        now,
		}
	case status.IsEvaluated() && hasOutcome && (runStatus == evalrun.StatusFailed || runStatus == evalrun.StatusRunning):
		return &mismatch{
			Kind: mismatchRunStatusMismatch, Severity: severityMedium,
			RecommendedAction: "audit run/status mismatch; manual confirmation required",
			DetectedAt:        now,
		}
	case status.IsFailed() && hasOutcome && runStatus == evalrun.StatusSucceeded:
		return &mismatch{
			Kind: mismatchTerminalConflict, Severity: severityHigh,
			RecommendedAction: "terminal conflict; require operator decision",
			DetectedAt:        now,
		}
	default:
		return nil
	}
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
