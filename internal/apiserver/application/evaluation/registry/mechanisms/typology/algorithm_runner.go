package typology

import (
	personalityadapter "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/runtime/adapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

type algorithmRunner struct {
	adapter          personalityadapter.ModelAdapter
	outcomeAssembler OutcomeAssembler
}

func algorithmRunnerFor(registry ModuleRegistry, algorithm modelcatalog.Algorithm) (algorithmRunner, error) {
	return registry.runnerFor(algorithm)
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
