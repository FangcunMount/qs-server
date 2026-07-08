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

// ReconcileService scans candidate assessments and repairs detected cross-store drift.
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

// ReconcileOnce lists pre-interpreted assessments, scans for drift, and repairs mismatches.
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
			assessment.StatusEvaluated.String(),
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

	repaired := 0
	for _, mismatch := range mismatches {
		observeMismatch(mismatch.Kind)

		var repairErr error
		switch mismatch.Kind {
		case MismatchReportWithoutInterpretedStatus:
			repairErr = s.reconciler.RepairInterpretedFinalization(ctx, mismatch.AssessmentID)
		case MismatchScoringArtifactWithoutEvaluatedStatus:
			repairErr = s.reconciler.RepairEvaluatedFinalization(ctx, mismatch.AssessmentID)
		default:
			continue
		}
		if repairErr != nil {
			observeRepair(mismatch.Kind, "error")
			log.Warnf("evaluation consistency repair failed (assessment_id=%d, kind=%s): %v",
				mismatch.AssessmentID, mismatch.Kind, repairErr)
			continue
		}
		observeRepair(mismatch.Kind, "success")
		repaired++
	}
	return repaired, nil
}
