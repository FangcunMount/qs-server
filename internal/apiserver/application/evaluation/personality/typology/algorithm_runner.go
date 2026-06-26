package typology

import (
	"fmt"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

var registeredAlgorithmRunners = map[assessmentmodel.Algorithm]algorithmRunner{
	assessmentmodel.AlgorithmMBTI: mbtiAlgorithmRunner{},
	assessmentmodel.AlgorithmSBTI: sbtiAlgorithmRunner{},
}

type algorithmRunner interface {
	algorithm() assessmentmodel.Algorithm
	buildOutcome(
		modelRef assessment.EvaluationModelRef,
		payload *modeltypology.Payload,
		sheet *port.AnswerSheetSnapshot,
	) (*assessment.AssessmentOutcome, error)
	buildReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error)
}

func algorithmRunnerFor(algorithm assessmentmodel.Algorithm) (algorithmRunner, error) {
	runner, ok := registeredAlgorithmRunners[algorithm]
	if !ok {
		return nil, fmt.Errorf("unsupported typology algorithm: %s", algorithm)
	}
	return runner, nil
}

type mbtiAlgorithmRunner struct{}

func (mbtiAlgorithmRunner) algorithm() assessmentmodel.Algorithm {
	return assessmentmodel.AlgorithmMBTI
}

func (mbtiAlgorithmRunner) buildOutcome(
	modelRef assessment.EvaluationModelRef,
	payload *modeltypology.Payload,
	sheet *port.AnswerSheetSnapshot,
) (*assessment.AssessmentOutcome, error) {
	return buildMBTIOutcome(modelRef, payload, sheet)
}

func (mbtiAlgorithmRunner) buildReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	return buildMBTIReport(outcome)
}

type sbtiAlgorithmRunner struct{}

func (sbtiAlgorithmRunner) algorithm() assessmentmodel.Algorithm {
	return assessmentmodel.AlgorithmSBTI
}

func (sbtiAlgorithmRunner) buildOutcome(
	modelRef assessment.EvaluationModelRef,
	payload *modeltypology.Payload,
	sheet *port.AnswerSheetSnapshot,
) (*assessment.AssessmentOutcome, error) {
	return buildSBTIOutcome(modelRef, payload, sheet)
}

func (sbtiAlgorithmRunner) buildReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	return buildSBTIReport(outcome)
}
