package pipeline

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

type AssessmentResultWriter interface {
	ApplyAndSave(ctx context.Context, evalCtx *Context) error
}

type repositoryAssessmentResultWriter struct {
	assessmentRepo assessment.Repository
}

func NewAssessmentResultWriter(assessmentRepo assessment.Repository) AssessmentResultWriter {
	return repositoryAssessmentResultWriter{assessmentRepo: assessmentRepo}
}

func (w repositoryAssessmentResultWriter) ApplyAndSave(ctx context.Context, evalCtx *Context) error {
	l := logger.L(ctx)
	if err := evalCtx.Assessment.ApplyEvaluation(evalCtx.EvaluationResult); err != nil {
		assessmentID, _ := evalCtx.Assessment.ID().Value()
		l.Errorw("Failed to apply evaluation result",
			"assessment_id", assessmentID,
			"error", err)
		return evalerrors.AssessmentInterpretFailed(err, "应用评估结果失败")
	}

	if err := w.assessmentRepo.Save(ctx, evalCtx.Assessment); err != nil {
		assessmentID, _ := evalCtx.Assessment.ID().Value()
		l.Errorw("Failed to save assessment",
			"assessment_id", assessmentID,
			"error", err)
		return evalerrors.Database(err, "保存测评失败")
	}
	assessmentID, _ := evalCtx.Assessment.ID().Value()
	l.Infow("Assessment saved successfully",
		"assessment_id", assessmentID)

	return nil
}
