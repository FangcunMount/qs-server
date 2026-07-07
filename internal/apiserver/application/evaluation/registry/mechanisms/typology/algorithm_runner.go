package typology

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	personalityadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/adapter"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type algorithmRunner struct {
	adapter          personalityadapter.ModelAdapter
	outcomeAssembler OutcomeAssembler
	reportRegistry   ReportAdapterRegistry
}

func algorithmRunnerFor(registry ModuleRegistry, algorithm modelcatalog.Algorithm) (algorithmRunner, error) {
	return registry.runnerFor(algorithm)
}

func (r algorithmRunner) algorithm() modelcatalog.Algorithm {
	if r.adapter == nil {
		return ""
	}
	return r.adapter.Algorithm()
}

func (r algorithmRunner) buildOutcome(
	modelRef assessment.EvaluationModelRef,
	payload *modeltypology.Payload,
	sheet *port.AnswerSheetSnapshot,
) (*assessment.AssessmentOutcome, error) {
	result, err := r.adapter.Score(payload, answerSheetFromPort(sheet))
	if err != nil {
		return nil, err
	}
	return r.outcomeAssembler.AssembleFromPayload(modelRef, payload, result)
}

func (r algorithmRunner) buildReport(outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	spec, mapping, decisionKind := resolveReportBuildContext(r, outcome)
	return r.reportRegistry.build(spec, mapping, decisionKind, outcome)
}
