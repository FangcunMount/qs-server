package execute

import (
	"context"
	"fmt"

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
		if latest != nil && latest.Attempt.Status == evalrun.StatusFailed && latest.Retryable() {
			attemptNo = latest.Attempt.Number + 1
		}
	}
	return evalrun.NewEvaluationRunWithAttempt(assessmentID, attemptNo), nil
}

func (s *service) persistEvaluationRun(ctx context.Context, run evalrun.EvaluationRun) error {
	if s == nil || s.runRepo == nil {
		return nil
	}
	return s.runRepo.Save(ctx, run)
}

func (s *service) persistEvaluationRunState(ctx context.Context, a *assessment.Assessment, run evalrun.EvaluationRun) error {
	if err := s.persistEvaluationRun(ctx, run); err != nil {
		if a != nil {
			s.failureFinalizer().MarkAsFailed(ctx, a, "评估运行记录保存失败: "+err.Error())
		}
		return fmt.Errorf("persist evaluation run: %w", err)
	}
	if s == nil || a == nil || s.runRepo == nil || s.assessmentRepo == nil || a.CurrentRunID() == "" {
		return nil
	}
	if err := s.assessmentRepo.Save(ctx, a); err != nil {
		s.failureFinalizer().MarkAsFailed(ctx, a, "当前运行ID保存失败: "+err.Error())
		return fmt.Errorf("persist current evaluation run id: %w", err)
	}
	return nil
}
