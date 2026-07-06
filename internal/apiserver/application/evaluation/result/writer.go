package result

import (
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

func NewWriter(
	assessmentRepo assessment.Repository,
	scoreProjectors ScoreProjectorRegistry,
	reportBuilders ReportBuilderRegistry,
	reportSaver ReportDurableSaver,
	notifier CompletionNotifier,
	reportStatus *reportstatus.Reporter,
) (Writer, error) {
	return interpretationreporting.NewWriter(assessmentRepo, scoreProjectors, reportBuilders, reportSaver, notifier, reportStatus)
}

func NewWriterWithEventAssemblers(
	assessmentRepo assessment.Repository,
	scoreProjectors ScoreProjectorRegistry,
	reportBuilders ReportBuilderRegistry,
	reportSaver ReportDurableSaver,
	notifier CompletionNotifier,
	reportStatus *reportstatus.Reporter,
	assemblers ...EventAssembler,
) (Writer, error) {
	return interpretationreporting.NewWriterWithEventAssemblers(
		assessmentRepo,
		scoreProjectors,
		reportBuilders,
		reportSaver,
		notifier,
		reportStatus,
		assemblers...,
	)
}

func ResolveOutcomeKey(outcome Outcome) evaluation.EvaluatorKey {
	return interpretationreporting.ResolveOutcomeKey(outcome)
}
