package typology

import (
	"fmt"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	personalityadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type algorithmRunner struct {
	adapter          personalityadapter.ModelAdapter
	outcomeAssembler outcomeAssemblerFunc
	reportBuilder    reportBuilderFunc
}

type reportBuilderFunc func(evaluationresult.Outcome) (*domainReport.InterpretReport, error)

type outcomeAssemblerFunc func(
	modelRef assessment.EvaluationModelRef,
	result evaluationtypology.ScoringResult,
) (*assessment.AssessmentOutcome, error)

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
	return r.outcomeAssembler(modelRef, result)
}

func (r algorithmRunner) buildReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	if r.reportBuilder == nil {
		return nil, fmt.Errorf("personality typology report builder is not configured")
	}
	return r.reportBuilder(outcome)
}
