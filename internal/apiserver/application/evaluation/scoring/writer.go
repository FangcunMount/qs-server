package scoring

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// Writer persists scoring outcomes and transitions Assessment to evaluated.
type Writer interface {
	Write(ctx context.Context, outcome evaluationresult.Outcome) error
}

type writer struct {
	assessmentRepo  assessment.Repository
	scoreProjectors evaluationresult.ScoreProjectorRegistry
}

// NewWriter creates a scoring outcome writer.
func NewWriter(
	assessmentRepo assessment.Repository,
	scoreProjectors evaluationresult.ScoreProjectorRegistry,
) Writer {
	return &writer{
		assessmentRepo:  assessmentRepo,
		scoreProjectors: scoreProjectors,
	}
}

func (w *writer) Write(ctx context.Context, outcome evaluationresult.Outcome) error {
	l := logger.L(ctx)
	if err := ensureScoringOutcome(outcome); err != nil {
		return evalerrors.AssessmentInterpretFailed(err, "应用计分结果失败")
	}
	if w.assessmentRepo == nil {
		return evalerrors.ModuleNotConfigured("assessment repository is not configured")
	}
	if err := outcome.Assessment.ApplyScoringOutcome(outcome.Execution); err != nil {
		return evalerrors.AssessmentInterpretFailed(err, "应用计分结果失败")
	}
	if err := w.assessmentRepo.Save(ctx, outcome.Assessment); err != nil {
		l.Errorw("Failed to save evaluated assessment",
			"assessment_id", outcome.Assessment.ID().Uint64(),
			"error", err)
		return evalerrors.Database(err, "保存计分结果失败")
	}
	if w.scoreProjectors != nil {
		key := evaluationresult.ResolveOutcomeKey(outcome)
		if projector := w.scoreProjectors.Resolve(key); projector != nil {
			if err := projector.Project(ctx, outcome); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureScoringOutcome(outcome evaluationresult.Outcome) error {
	if outcome.Assessment == nil {
		return fmt.Errorf("assessment is required")
	}
	if outcome.Execution == nil {
		return fmt.Errorf("evaluation outcome is required")
	}
	if !outcome.Assessment.Status().CanApplyScoring() {
		return assessment.NewInvalidStatusError("apply scoring", outcome.Assessment.Status())
	}
	modelRef := outcome.Assessment.EvaluationModelRef()
	if modelRef == nil || modelRef.IsEmpty() {
		return assessment.ErrNoEvaluationModel
	}
	if outcome.Execution.ModelRef.IsEmpty() {
		outcome.Execution.ModelRef = *modelRef
		return nil
	}
	if !modelRef.SameIdentity(outcome.Execution.ModelRef) {
		return assessment.ErrEvaluationModelMismatch
	}
	return nil
}
