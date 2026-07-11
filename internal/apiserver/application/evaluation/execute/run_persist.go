package execute

import (
	"context"
	"fmt"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

func (s *service) newEvaluationRun(ctx context.Context, assessmentID uint64) (evalrun.EvaluationRun, error) {
	attemptNo := 1
	if s.runRepo != nil {
		latest, err := s.runRepo.FindLatestByAssessmentID(ctx, assessmentID)
		if err != nil {
			return evalrun.EvaluationRun{}, fmt.Errorf("load latest evaluation run: %w", err)
		}
		if latest != nil {
			switch latest.Attempt.Status {
			case evalrun.StatusPending, evalrun.StatusRunning:
				return *latest, nil
			case evalrun.StatusFailed:
				if !latest.Retryable() {
					return evalrun.EvaluationRun{}, fmt.Errorf("latest evaluation run %s is not retryable", latest.RunID)
				}
				attemptNo = latest.Attempt.Number + 1
			case evalrun.StatusSucceeded:
				return evalrun.EvaluationRun{}, fmt.Errorf("latest evaluation run %s already succeeded", latest.RunID)
			default:
				return evalrun.EvaluationRun{}, fmt.Errorf("latest evaluation run %s has unknown status %q", latest.RunID, latest.Attempt.Status)
			}
		}
	}
	return evalrun.NewEvaluationRunWithAttempt(assessmentID, attemptNo), nil
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
		if err := s.runRepo.Save(txCtx, run); err != nil {
			return fmt.Errorf("persist evaluation run: %w", err)
		}
		if err := s.assessmentRepo.Save(txCtx, a); err != nil {
			return fmt.Errorf("persist current evaluation run id: %w", err)
		}
		return nil
	})
}
