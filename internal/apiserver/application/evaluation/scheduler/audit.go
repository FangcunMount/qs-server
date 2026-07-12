// Package scheduler contains read-only Evaluation maintenance use cases.
package scheduler

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gorm.io/gorm"
)

type MismatchKind string

const MismatchOutcomeWithoutEvaluatedStatus MismatchKind = "outcome_without_evaluated_status"

type Mismatch struct {
	AssessmentID uint64
	Kind         MismatchKind
	DetectedAt   time.Time
}

type Service interface {
	AuditOnce(context.Context, int) (int, error)
}
type service struct {
	assessments domainassessment.Repository
	outcomes    domainoutcome.Repository
	reader      evaluationreadmodel.AssessmentReader
}

func NewService(assessments domainassessment.Repository, outcomes domainoutcome.Repository, reader evaluationreadmodel.AssessmentReader) Service {
	return &service{assessments: assessments, outcomes: outcomes, reader: reader}
}

func (s *service) AuditOnce(ctx context.Context, limit int) (int, error) {
	if s == nil || s.assessments == nil || s.reader == nil {
		return 0, fmt.Errorf("evaluation consistency audit is not configured")
	}
	if limit <= 0 {
		return 0, nil
	}
	rows, _, err := s.reader.ListAssessments(ctx, evaluationreadmodel.AssessmentFilter{Statuses: []string{domainassessment.StatusSubmitted.String()}}, evaluationreadmodel.PageRequest{Page: 1, PageSize: limit})
	if err != nil {
		return 0, err
	}
	detected := 0
	for _, row := range rows {
		if row.ID == 0 {
			continue
		}
		mismatch, scanErr := s.scanOne(ctx, row.ID)
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
	return detected, nil
}

func (s *service) scanOne(ctx context.Context, assessmentID uint64) (*Mismatch, error) {
	a, err := s.assessments.FindByID(ctx, domainassessment.NewID(assessmentID))
	if err != nil || a == nil {
		return nil, err
	}
	if s.outcomes == nil || !a.Status().IsSubmitted() {
		return nil, nil
	}
	record, err := s.outcomes.FindByAssessmentID(ctx, meta.FromUint64(assessmentID))
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	return &Mismatch{AssessmentID: assessmentID, Kind: MismatchOutcomeWithoutEvaluatedStatus, DetectedAt: time.Now()}, nil
}

var (
	evaluationConsistencyMismatchTotal    = promauto.NewCounterVec(prometheus.CounterOpts{Namespace: "qs", Subsystem: "evaluation_consistency", Name: "mismatch_total", Help: "Total evaluation cross-store mismatches detected by the consistency reconciler."}, []string{"kind"})
	evaluationConsistencyDispositionTotal = promauto.NewCounterVec(prometheus.CounterOpts{Namespace: "qs", Subsystem: "evaluation_consistency", Name: "disposition_total", Help: "Total evaluation consistency mismatches by kind and audit disposition."}, []string{"kind", "disposition"})
)

func observeMismatch(kind MismatchKind) {
	evaluationConsistencyMismatchTotal.WithLabelValues(string(kind)).Inc()
}
func observeDisposition(kind MismatchKind, disposition string) {
	evaluationConsistencyDispositionTotal.WithLabelValues(string(kind), disposition).Inc()
}
