package result

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type Outcome = evaloutcome.Outcome

// LegacyResult projects the canonical outcome into the legacy write model.
func LegacyResult(o Outcome) *assessment.EvaluationResult {
	return evaloutcome.LegacyResult(o)
}

// NewOutcomeFromLegacyResult adapts a legacy evaluation result for tests and compatibility callers.
func NewOutcomeFromLegacyResult(a *assessment.Assessment, input *evaluationinput.InputSnapshot, result *assessment.EvaluationResult) Outcome {
	return evaloutcome.NewOutcomeFromLegacyResult(a, input, result)
}

type Writer = interpretationreporting.Writer

type ScoreProjector = interpretationreporting.ScoreProjector

type ScoreProjectorRegistry = interpretationreporting.ScoreProjectorRegistry

type ReportBuilder = interpretationreporting.ReportBuilder

type ReportBuilderRegistry = interpretationreporting.ReportBuilderRegistry

type ReportDurableSaver = interpretationreporting.ReportDurableSaver

type ReportDurableWriter = interpretationreporting.ReportDurableWriter

type ReportEventStager = interpretationreporting.ReportEventStager

type EventAssembler = interpretationreporting.EventAssembler

type EventAssemblerRegistry = interpretationreporting.EventAssemblerRegistry

type CompletionNotifier = interpretationreporting.CompletionNotifier
