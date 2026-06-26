package typology

import (
	"fmt"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	personalityadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type algorithmRunner struct {
	adapter       personalityadapter.ModelAdapter
	reportBuilder reportBuilderFunc
}

type reportBuilderFunc func(evaluationresult.Outcome) (*domainReport.InterpretReport, error)

var reportBuilders = map[assessmentmodel.Algorithm]reportBuilderFunc{
	assessmentmodel.AlgorithmMBTI: buildMBTIReport,
	assessmentmodel.AlgorithmSBTI: buildSBTIReport,
}

func algorithmRunnerFor(algorithm assessmentmodel.Algorithm) (algorithmRunner, error) {
	adapter, err := personalityadapter.DefaultRegistry().Resolve(algorithm)
	if err != nil {
		return algorithmRunner{}, err
	}
	reportBuilder, ok := reportBuilders[algorithm]
	if !ok {
		return algorithmRunner{}, fmt.Errorf("unsupported typology algorithm: %s", algorithm)
	}
	return algorithmRunner{
		adapter:       adapter,
		reportBuilder: reportBuilder,
	}, nil
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
	return r.adapter.BuildOutcome(modelRef, payload, answerSheetFromPort(sheet))
}

func (r algorithmRunner) buildReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	if r.reportBuilder == nil {
		return nil, fmt.Errorf("personality typology report builder is not configured")
	}
	return r.reportBuilder(outcome)
}
