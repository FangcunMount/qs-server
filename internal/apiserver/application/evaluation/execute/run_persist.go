package execute

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

func (s *service) persistEvaluationRun(ctx context.Context, run evalrun.EvaluationRun) error {
	if s == nil || s.runRepo == nil {
		return nil
	}
	return s.runRepo.Save(ctx, run)
}

func (s *service) persistEvaluationRunState(ctx context.Context, a *assessment.Assessment, run evalrun.EvaluationRun) {
	if err := s.persistEvaluationRun(ctx, run); err != nil && a != nil {
		s.failureFinalizer().MarkAsFailed(ctx, a, "评估运行记录保存失败: "+err.Error())
		return
	}
	if a == nil || s.runRepo == nil || s.assessmentRepo == nil || a.CurrentRunID() == "" {
		return
	}
	if err := s.assessmentRepo.Save(ctx, a); err != nil {
		s.failureFinalizer().MarkAsFailed(ctx, a, "当前运行ID保存失败: "+err.Error())
	}
}
