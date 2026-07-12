package consistency

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

// Service is the narrow port used by background schedulers.
type Service interface {
	ReconcileOnce(ctx context.Context, limit int) (int, error)
}

// ReconcileService scans candidate assessments and reports historical drift.
type ReconcileService struct {
	reconciler *Reconciler
	reader     evaluationreadmodel.AssessmentReader
}

// NewReconcileService creates the production consistency reconcile orchestrator.
func NewReconcileService(reconciler *Reconciler, reader evaluationreadmodel.AssessmentReader) *ReconcileService {
	return &ReconcileService{
		reconciler: reconciler,
		reader:     reader,
	}
}

// ReconcileOnce scans submitted assessments for incomplete Evaluation finalization.
func (s *ReconcileService) ReconcileOnce(ctx context.Context, limit int) (int, error) {
	if s == nil || s.reconciler == nil {
		return 0, fmt.Errorf("evaluation consistency reconcile service is not configured")
	}
	if s.reader == nil {
		return 0, fmt.Errorf("evaluation consistency reconcile candidate reader is not configured")
	}
	if limit <= 0 {
		return 0, nil
	}

	rows, _, err := s.reader.ListAssessments(ctx, evaluationreadmodel.AssessmentFilter{
		Statuses: []string{
			assessment.StatusSubmitted.String(),
		},
	}, evaluationreadmodel.PageRequest{Page: 1, PageSize: limit})
	if err != nil {
		return 0, err
	}

	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		if row.ID > 0 {
			ids = append(ids, row.ID)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}

	mismatches, err := s.reconciler.Scan(ctx, ids)
	if err != nil {
		return 0, err
	}

	detected := 0
	for _, mismatch := range mismatches {
		observeMismatch(mismatch.Kind)
		observeRepair(mismatch.Kind, "deferred")
		log.Warnf("evaluation consistency drift requires audited migration (assessment_id=%d, kind=%s)",
			mismatch.AssessmentID, mismatch.Kind)
		detected++
	}
	return detected, nil
}
