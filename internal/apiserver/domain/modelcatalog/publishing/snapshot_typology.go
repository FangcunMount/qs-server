package publishing

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

func buildTypologyPublishedSnapshot(model *AssessmentModel) (*PublishedModelSnapshot, error) {
	if model.Kind != binding.KindPersonality {
		return nil, fmt.Errorf("model kind %s is not personality", model.Kind)
	}
	if model.SubKind != binding.SubKindTypology {
		return nil, fmt.Errorf("personality model sub_kind %s is not typology", model.SubKind)
	}
	if model.Definition.IsEmpty() {
		return nil, fmt.Errorf("personality model definition is empty")
	}
	payload, runtime, err := TypologyPayloadAndRuntimeSpecFromModel(model)
	if err != nil {
		return nil, err
	}
	prepareTypologyPublishedPayload(payload, model, runtime)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal typology payload: %w", err)
	}
	decisionKind := runtime.Decision.Kind
	if decisionKind == "" {
		return nil, fmt.Errorf("runtime decision.kind is required for publish")
	}
	return &PublishedModelSnapshot{
		SchemaVersion: SchemaVersionV2,
		PayloadFormat: PayloadFormatPersonalityTypologyV1,
		Model: ModelDefinition{
			ProductChannel: binding.ResolveProductChannel(model.Kind, model.ProductChannel),
			Kind:           binding.KindPersonality,
			SubKind:        binding.SubKindTypology,
			Algorithm:      model.Algorithm,
			Code:           model.Code,
			Version:        modelVersionString(model),
			Title:          model.Title,
			Status:         string(ModelStatusPublished),
		},
		Binding:  model.Binding,
		Decision: DecisionSpec{Kind: decisionKind},
		Source:   SourceRef{},
		Payload:  encoded,
	}, nil
}

func prepareTypologyPublishedPayload(payload *typology.Payload, model *AssessmentModel, runtime *typology.RuntimeSpec) {
	payload.Code = model.Code
	payload.Version = modelVersionString(model)
	payload.Title = model.Title
	payload.QuestionnaireCode = model.Binding.QuestionnaireCode
	payload.QuestionnaireVersion = model.Binding.QuestionnaireVersion
	payload.Status = string(ModelStatusPublished)
	payload.Algorithm = model.Algorithm
	payload.Runtime = runtime
}
