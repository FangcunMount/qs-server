package typology

import (
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	personalityadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type algorithmRunner struct {
	adapter          personalityadapter.ModelAdapter
	outcomeAssembler OutcomeAssembler
	reportRegistry   ReportAdapterRegistry
}

func algorithmRunnerFor(registry ModuleRegistry, algorithm assessmentmodel.Algorithm) (algorithmRunner, error) {
	return registry.runnerFor(algorithm)
}

func (r algorithmRunner) algorithm() assessmentmodel.Algorithm {
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

func (r algorithmRunner) buildReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	spec, mapping, decisionKind := resolveReportBuildContext(r, outcome)
	return r.reportRegistry.build(spec, mapping, decisionKind, outcome)
}
