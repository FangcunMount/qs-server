package publishedmodel

import (
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func buildTypology(model *domain.AssessmentModel) (*port.AssessmentSnapshot, error) {
	if !domain.IsTypologyKind(model.Kind) {
		return nil, fmt.Errorf("model kind %s is not typology", model.Kind)
	}
	if model.SubKind != domain.SubKindTypology {
		return nil, fmt.Errorf("typology model sub_kind %s is not typology", model.SubKind)
	}
	if model.Definition.IsEmpty() {
		return nil, fmt.Errorf("typology model definition is empty")
	}
	if model.DefinitionV2 == nil {
		return nil, fmt.Errorf("typology definition_v2 is required")
	}
	payload, runtime, err := typology.PayloadAndRuntimeSpecFromDefinition(model.Definition.Data, model.Algorithm)
	if err != nil {
		return nil, err
	}
	prepareTypologyPayload(payload, model, runtime)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal typology payload: %w", err)
	}
	decisionKind, err := model.DecisionKindForDefinition()
	if err != nil {
		return nil, err
	}
	return recordFromModel(model, domain.KindTypology, domain.SubKindTypology, model.Algorithm, domain.PayloadFormatPersonalityTypologyV1, decisionKind, encoded), nil
}

func prepareTypologyPayload(payload *typology.Payload, model *domain.AssessmentModel, runtime *typology.RuntimeSpec) {
	payload.Code = model.Code
	payload.Version = modelVersionString(model)
	payload.Title = model.Title
	payload.QuestionnaireCode = model.Binding.QuestionnaireCode
	payload.QuestionnaireVersion = model.Binding.QuestionnaireVersion
	payload.Status = string(domain.ModelStatusPublished)
	payload.Algorithm = model.Algorithm
	payload.Runtime = runtime
}
