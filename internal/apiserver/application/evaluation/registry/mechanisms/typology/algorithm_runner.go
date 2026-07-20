package typology

import (
	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	personalityadapter "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/runtime/adapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

type algorithmRunner struct {
	adapter          personalityadapter.ModelAdapter
	outcomeAssembler OutcomeAssembler
}

func (r algorithmRunner) buildOutcome(
	modelRef assessment.EvaluationModelRef,
	input *port.InputSnapshot,
	payload *modeltypology.Payload,
	sheet *port.AnswerSheetSnapshot,
) (*domainoutcome.Execution, error) {
	def, _ := port.DefinitionV2FromSnapshot(input)
	result, err := scoreTypology(r.adapter, payload, def, answerSheetFromPort(sheet))
	if err != nil {
		return nil, err
	}
	spec, err := modeltypology.ResolveRuntimeSpec(def, payload)
	if err != nil {
		return nil, err
	}
	return r.outcomeAssembler.Assemble(modelRef, result, spec.OutcomeMapping)
}

func scoreTypology(
	adapter personalityadapter.ModelAdapter,
	payload *modeltypology.Payload,
	def *modeldefinition.Definition,
	sheet *evalinput.AnswerSheet,
) (outcometypology.ScoringResult, error) {
	if def != nil {
		if canonical, ok := adapter.(personalityadapter.CanonicalScorer); ok {
			return canonical.ScoreWithDefinition(payload, def, sheet)
		}
	}
	return adapter.Score(payload, sheet)
}
