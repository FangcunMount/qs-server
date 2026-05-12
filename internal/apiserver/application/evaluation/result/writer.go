package result

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

type writer struct {
	assessmentRepo  assessment.Repository
	scoreProjectors ScoreProjectorRegistry
	reportBuilders  ReportBuilderRegistry
	reportSaver     ReportDurableSaver
	eventAssembler  EventAssembler
	notifier        CompletionNotifier
}

func NewWriter(
	assessmentRepo assessment.Repository,
	scoreProjectors ScoreProjectorRegistry,
	reportBuilders ReportBuilderRegistry,
	reportSaver ReportDurableSaver,
	notifier CompletionNotifier,
) Writer {
	return &writer{
		assessmentRepo:  assessmentRepo,
		scoreProjectors: scoreProjectors,
		reportBuilders:  reportBuilders,
		reportSaver:     reportSaver,
		eventAssembler:  NewEventAssembler(),
		notifier:        notifier,
	}
}

func (w *writer) Write(ctx context.Context, outcome Outcome) error {
	l := logger.L(ctx)
	if outcome.Assessment == nil {
		return evalerrors.ModuleNotConfigured("assessment is required for evaluation result writer")
	}
	if outcome.Result == nil {
		return evalerrors.ModuleNotConfigured("evaluation result is required for evaluation result writer")
	}
	kind := resolveOutcomeKind(outcome)

	if w.scoreProjectors != nil {
		if projector := w.scoreProjectors.Resolve(kind); projector != nil {
			if err := projector.Project(ctx, outcome); err != nil {
				return err
			}
		}
	}

	if err := outcome.Assessment.ApplyEvaluation(outcome.Result); err != nil {
		l.Errorw("Failed to apply evaluation result",
			"assessment_id", outcome.Assessment.ID().Uint64(),
			"error", err)
		return evalerrors.AssessmentInterpretFailed(err, "应用评估结果失败")
	}
	if w.assessmentRepo == nil {
		return evalerrors.ModuleNotConfigured("assessment repository is not configured")
	}
	if err := w.assessmentRepo.Save(ctx, outcome.Assessment); err != nil {
		l.Errorw("Failed to save assessment",
			"assessment_id", outcome.Assessment.ID().Uint64(),
			"error", err)
		return evalerrors.Database(err, "保存测评失败")
	}

	if w.reportSaver == nil {
		return evalerrors.ModuleNotConfigured("report durable saver is not configured")
	}
	if w.reportBuilders == nil {
		return evalerrors.ModuleNotConfigured("evaluation report builder registry is not configured")
	}
	builder, err := w.reportBuilders.Resolve(kind)
	if err != nil {
		return err
	}
	rpt, err := builder.Build(ctx, outcome)
	if err != nil {
		return evalerrors.AssessmentInterpretFailed(err, "生成报告失败")
	}
	if err := w.reportSaver.SaveReportDurably(ctx, rpt, outcome.Assessment.TesteeID(), w.eventAssembler.BuildSuccessEvents(outcome, rpt)); err != nil {
		return evalerrors.Database(err, "保存报告失败")
	}

	if w.notifier != nil {
		w.notifier.NotifyCompletion(ctx, outcome)
	}
	return nil
}

func resolveOutcomeKind(outcome Outcome) assessment.EvaluationModelKind {
	if !outcome.Result.ModelRef.IsEmpty() {
		return outcome.Result.ModelRef.Kind()
	}
	if outcome.Assessment != nil && outcome.Assessment.EvaluationModelRef() != nil {
		return outcome.Assessment.EvaluationModelRef().Kind()
	}
	if outcome.Input != nil && outcome.Input.Model != nil {
		return assessment.EvaluationModelKind(outcome.Input.Model.Kind)
	}
	return ""
}
