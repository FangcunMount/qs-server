package execute

import (
	"context"
	"fmt"
	"time"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
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
	request := evaluationrun.ClaimRequest{
		AssessmentID: assessmentID,
		Token:        claimToken,
		ClaimedAt:    now,
		LeaseUntil:   now.Add(lease),
		TraceID:      traceID,
	}
	if authorization, ok := retrygovernance.AuthorizationFromContext(ctx); ok {
		request.RetryEventID = authorization.EventID
		request.ExpectedAttempt = authorization.ExpectedAttempt
		request.Origin = authorization.Origin
		request.ActionRequestID = authorization.ActionRequestID
	}
	result, err := s.runRepo.Claim(ctx, request)
	if err != nil {
		return evaluationrun.ClaimResult{}, fmt.Errorf("claim evaluation run: %w", err)
	}
	return result, nil
}

func (s *service) persistClaimedEvaluationRun(ctx context.Context, run evalrun.EvaluationRun) error {
	if s == nil || s.runRepo == nil {
		return evalerrors.ModuleNotConfigured("evaluation run repository is not configured")
	}
	if run.ID() == "" {
		return fmt.Errorf("evaluation run id is required")
	}
	if err := s.runRepo.SaveClaimed(ctx, run); err != nil {
		return fmt.Errorf("persist evaluation run: %w", err)
	}
	return nil
}
