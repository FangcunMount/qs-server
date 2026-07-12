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
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type mismatchKind string

const mismatchOutcomeWithoutEvaluatedStatus mismatchKind = "outcome_without_evaluated_status"

type mismatch struct {
	AssessmentID uint64
	Kind         mismatchKind
	DetectedAt   time.Time
}

type Service interface {
	AuditOnce(context.Context, int) (int, error)
}

// SubmittedCandidateReader is the scheduler-specific keyset scan port. It has
// no user-facing pagination semantics.
type SubmittedCandidateReader interface {
	ListSubmittedAssessmentIDsAfter(context.Context, uint64, int) ([]uint64, error)
}

type service struct {
	assessments domainassessment.Repository
	outcomes    domainoutcome.Repository
	reader      SubmittedCandidateReader
	mu          sync.Mutex
	cursor      uint64
}

func NewService(assessments domainassessment.Repository, outcomes domainoutcome.Repository, reader SubmittedCandidateReader) Service {
	return &service{assessments: assessments, outcomes: outcomes, reader: reader}
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
		mismatch, scanErr := s.scanOne(ctx, assessmentID)
		if scanErr != nil {
			return 0, scanErr
		}
		if mismatch == nil {
			continue
		}
		observeMismatch(mismatch.Kind)
		observeDisposition(mismatch.Kind, "deferred")
		log.Warnf("evaluation consistency drift requires audited migration (assessment_id=%d, kind=%s)", mismatch.AssessmentID, mismatch.Kind)
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
	if !a.Status().IsSubmitted() {
		return nil, nil
	}
	record, err := s.outcomes.FindByAssessmentID(ctx, meta.FromUint64(assessmentID))
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	return &mismatch{AssessmentID: assessmentID, Kind: mismatchOutcomeWithoutEvaluatedStatus, DetectedAt: time.Now()}, nil
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
