package scoring

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// Writer 持久化计分结果 和 transitions Assessment 到 evaluated。
type Writer interface {
	Write(ctx context.Context, outcome evaloutcome.Outcome) error
}

type writer struct {
	assessmentRepo  assessment.Repository
	scoreProjectors interpretationreporting.ScoreProjectorRegistry
	snapshotStore   ScoringSnapshotStore
}

// NewWriter 创建计分结果写入器。
func NewWriter(
	assessmentRepo assessment.Repository,
	scoreProjectors interpretationreporting.ScoreProjectorRegistry,
	snapshotStore ScoringSnapshotStore,
) Writer {
	return &writer{
		assessmentRepo:  assessmentRepo,
		scoreProjectors: scoreProjectors,
		snapshotStore:   snapshotStore,
	}
}

func (w *writer) Write(ctx context.Context, outcome evaloutcome.Outcome) error {
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
	if w.snapshotStore != nil {
		if err := w.snapshotStore.Save(ctx, outcome.Assessment.ID().Uint64(), outcome.Execution); err != nil {
			return evalerrors.Database(err, "保存计分快照失败")
		}
	}
	if w.scoreProjectors != nil {
		mechanismKey, ok := interpretationreporting.MechanismReportBuilderKeyFromOutcome(outcome)
		if ok {
			projector := w.scoreProjectors.ResolveByMechanism(mechanismKey)
			if err := projector.Project(ctx, outcome); err != nil {
				return err
			}
		} else {
			key := interpretationreporting.ResolveOutcomeKey(outcome)
			if projector := w.scoreProjectors.Resolve(key); projector != nil {
				if err := projector.Project(ctx, outcome); err != nil {
					return err
				}
			}
		}
	}
	if err := w.assessmentRepo.Save(ctx, outcome.Assessment); err != nil {
		l.Errorw("Failed to save evaluated assessment",
			"assessment_id", outcome.Assessment.ID().Uint64(),
			"error", err)
		return evalerrors.Database(err, "保存计分结果失败")
	}
	return nil
}

func ensureScoringOutcome(outcome evaloutcome.Outcome) error {
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
