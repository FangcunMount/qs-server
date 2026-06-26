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
	switch algorithm {
	case assessmentmodel.AlgorithmMBTI:
		return mbtiAlgorithmRunner{}, nil
	case assessmentmodel.AlgorithmSBTI:
		return sbtiAlgorithmRunner{}, nil
	default:
		return nil, fmt.Errorf("unsupported typology algorithm: %s", algorithm)
	}
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
