package execute

import (
	"context"
	"fmt"
	"time"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
)

func (s *service) claimEvaluationRun(
	ctx context.Context,
	assessmentID uint64,
	claimToken string,
	traceID string,
	now time.Time,
) (evaluationrun.ClaimResult, error) {
	if s == nil || s.runRepo == nil {
		return evaluationrun.ClaimResult{}, evalerrors.ModuleNotConfigured("evaluation run repository is not configured")
	}
	lease := s.runLease
	if lease <= 0 {
		lease = defaultEvaluationRunLease
	}
	result, err := s.runRepo.Claim(ctx, evaluationrun.ClaimRequest{
		AssessmentID: assessmentID,
		Token:        claimToken,
		ClaimedAt:    now,
		LeaseUntil:   now.Add(lease),
		TraceID:      traceID,
	})
	if err != nil {
		return evaluationrun.ClaimResult{}, fmt.Errorf("claim evaluation run: %w", err)
	}
	return result, nil
}

func (s *service) persistStartedEvaluationRun(ctx context.Context, a *assessment.Assessment, run evalrun.EvaluationRun) error {
	if s == nil || s.txRunner == nil || s.runRepo == nil || s.assessmentRepo == nil {
		return evalerrors.ModuleNotConfigured("evaluation run lifecycle requires transaction, assessment and run dependencies")
	}
	if a == nil {
		return fmt.Errorf("assessment is required")
	}
	if run.RunID == "" || a.CurrentRunID() != run.RunID {
		return fmt.Errorf("assessment current run does not match evaluation run")
	}
	return s.txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.runRepo.SaveClaimed(txCtx, run); err != nil {
			return fmt.Errorf("persist evaluation run: %w", err)
		}
		if err := s.assessmentRepo.Save(txCtx, a); err != nil {
			return fmt.Errorf("persist current evaluation run id: %w", err)
		}
		return nil
	})
}
