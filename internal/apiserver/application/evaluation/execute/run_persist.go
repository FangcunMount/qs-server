package execute

import (
	"context"
	"fmt"
	"time"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
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

func (s *service) persistClaimedEvaluationRun(ctx context.Context, run evalrun.EvaluationRun) error {
	if s == nil || s.runRepo == nil {
		return evalerrors.ModuleNotConfigured("evaluation run repository is not configured")
	}
	if run.RunID == "" {
		return fmt.Errorf("evaluation run id is required")
	}
	if err := s.runRepo.SaveClaimed(ctx, run); err != nil {
		return fmt.Errorf("persist evaluation run: %w", err)
	}
	return nil
}
