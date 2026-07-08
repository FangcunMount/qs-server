package writer

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	evaluationapp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/projection"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type writer struct {
	assessmentRepo  assessment.Repository
	scoreProjectors projection.ScoreProjectorRegistry
	reportBuilders  registry.ReportBuilderRegistry
	reportSaver     ReportDurableSaver
	eventAssemblers projection.EventAssemblerRegistry
	notifier        CompletionNotifier
	reportStatus    *reportstatus.Reporter
}

// NewWriter 创建解释写入器 用于之后 计分 completes。
func NewWriter(
	assessmentRepo assessment.Repository,
	scoreProjectors projection.ScoreProjectorRegistry,
	reportBuilders registry.ReportBuilderRegistry,
	reportSaver ReportDurableSaver,
	notifier CompletionNotifier,
	reportStatus *reportstatus.Reporter,
) (Writer, error) {
	return NewWriterWithEventAssemblers(
		assessmentRepo,
		scoreProjectors,
		reportBuilders,
		reportSaver,
		notifier,
		reportStatus,
		projection.DefaultMechanismEventAssemblers()...,
	)
}

// NewWriterWithEventAssemblers 允许显式 事件组装器 registration in tests。
func NewWriterWithEventAssemblers(
	assessmentRepo assessment.Repository,
	scoreProjectors projection.ScoreProjectorRegistry,
	reportBuilders registry.ReportBuilderRegistry,
	reportSaver ReportDurableSaver,
	notifier CompletionNotifier,
	reportStatus *reportstatus.Reporter,
	assemblers ...projection.EventAssembler,
) (Writer, error) {
	eventAssemblers, err := projection.NewEventAssemblerRegistry(assemblers...)
	if err != nil {
		return nil, err
	}
	return &writer{
		assessmentRepo:  assessmentRepo,
		scoreProjectors: scoreProjectors,
		reportBuilders:  reportBuilders,
		reportSaver:     reportSaver,
		eventAssemblers: eventAssemblers,
		notifier:        notifier,
		reportStatus:    reportStatus,
	}, nil
}

type preparedOutcome struct {
	projector projection.ScoreProjector
	report    *domainReport.InterpretReport
	events    []event.DomainEvent
}

func (w *writer) Write(ctx context.Context, outcome evaloutcome.Outcome) error {
	l := logger.L(ctx)
	if outcome.Assessment == nil {
		return evalerrors.ModuleNotConfigured("assessment is required for interpretation writer")
	}
	if outcome.Execution == nil {
		return evalerrors.ModuleNotConfigured("evaluation outcome is required for interpretation writer")
	}
	if w.reportSaver == nil {
		return evalerrors.ModuleNotConfigured("report durable saver is not configured")
	}

	prepared, err := w.prepare(ctx, outcome)
	if err != nil {
		return err
	}

	if err := w.reportSaver.SaveReportDurably(ctx, prepared.report, outcome.Assessment.TesteeID(), prepared.events); err != nil {
		return evalerrors.Database(err, "保存报告失败")
	}

	if prepared.projector != nil && outcome.Assessment.Status().IsSubmitted() {
		if err := prepared.projector.Project(ctx, outcome); err != nil {
			return err
		}
	}

	if err := outcome.Assessment.ApplyOutcome(outcome.Execution); err != nil {
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

	if w.notifier != nil {
		w.notifier.NotifyCompletion(ctx, outcome)
	}
	if w.reportStatus != nil && prepared.report != nil {
		assessmentID, answerSheetID := evaluationapp.ReportStatusIDs(outcome.Assessment)
		w.reportStatus.SetCompleted(ctx, assessmentID, answerSheetID, prepared.report.ID().String())
	}
	return nil
}

func (w *writer) prepare(ctx context.Context, outcome evaloutcome.Outcome) (preparedOutcome, error) {
	if err := ensureOutcomeCanApplyEvaluation(outcome); err != nil {
		return preparedOutcome{}, evalerrors.AssessmentInterpretFailed(err, "应用评估结果失败")
	}
	mechanismKey, ok := registry.MechanismReportBuilderKeyFromOutcome(outcome)
	if !ok {
		return preparedOutcome{}, fmt.Errorf("unsupported mechanism report builder key for outcome")
	}
	var projector projection.ScoreProjector
	if w.scoreProjectors != nil {
		projector = w.scoreProjectors.ResolveByMechanism(mechanismKey)
	}
	if w.reportBuilders == nil {
		return preparedOutcome{}, evalerrors.ModuleNotConfigured("interpretation report builder registry is not configured")
	}
	builder, err := w.reportBuilders.ResolveByMechanism(mechanismKey)
	if err != nil {
		return preparedOutcome{}, err
	}
	rpt, err := builder.Build(ctx, outcome)
	if err != nil {
		return preparedOutcome{}, evalerrors.AssessmentInterpretFailed(err, "生成报告失败")
	}
	assembler := w.eventAssemblers.ResolveByMechanism(mechanismKey)
	return preparedOutcome{
		projector: projector,
		report:    rpt,
		events:    assembler.BuildSuccessEvents(outcome, rpt),
	}, nil
}

func ensureOutcomeCanApplyEvaluation(outcome evaloutcome.Outcome) error {
	if outcome.Assessment == nil {
		return fmt.Errorf("assessment is required")
	}
	if outcome.Execution == nil {
		return fmt.Errorf("evaluation outcome is required")
	}
	if !outcome.Assessment.Status().CanApplyInterpretation() {
		return assessment.NewInvalidStatusError("apply evaluation", outcome.Assessment.Status())
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

// ResolveOutcomeKey 解析评估器键 从 结果。
func ResolveOutcomeKey(outcome evaloutcome.Outcome) evaluation.ExecutionIdentity {
	if outcome.Execution != nil && !outcome.Execution.ModelRef.IsEmpty() {
		return outcome.Execution.ModelRef.ExecutionIdentity()
	}
	if outcome.Assessment != nil && outcome.Assessment.EvaluationModelRef() != nil {
		return outcome.Assessment.EvaluationModelRef().ExecutionIdentity()
	}
	if outcome.Input != nil && outcome.Input.Model != nil {
		return outcome.Input.Model.ModelRef().ExecutionIdentity()
	}
	return evaluation.ExecutionIdentity{}
}
