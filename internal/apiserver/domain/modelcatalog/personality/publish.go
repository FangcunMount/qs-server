package personality

import (
	"encoding/json"
	"fmt"
	"strconv"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// BuildPublishedSnapshot materializes a v2 published snapshot from a draft personality model.
func BuildPublishedSnapshot(model *domain.AssessmentModel) (*domain.PublishedModelSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	if model.Kind != domain.KindPersonality {
		return nil, fmt.Errorf("model kind %s is not personality", model.Kind)
	}
	if model.SubKind != domain.SubKindTypology {
		return nil, fmt.Errorf("personality model sub_kind %s is not typology", model.SubKind)
	}
	if model.Definition.IsEmpty() {
		return nil, fmt.Errorf("personality model definition is empty")
	}

	payload, runtime, err := PayloadAndRuntimeSpecFromModel(model)
	if err != nil {
		return nil, err
	}
	preparePublishedPayload(payload, model, runtime)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal typology payload: %w", err)
	}
	decisionKind := runtime.Decision.Kind
	if decisionKind == "" {
		return nil, fmt.Errorf("runtime decision.kind is required for publish")
	}
	return &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Model: domain.ModelDefinition{
			ProductChannel: domain.ResolveProductChannel(model.Kind, model.ProductChannel),
			Kind:           domain.KindPersonality,
			SubKind:        domain.SubKindTypology,
			Algorithm:      model.Algorithm,
			Code:           model.Code,
			Version:        modelVersionString(model),
			Title:          model.Title,
			Status:         string(domain.ModelStatusPublished),
		},
		Binding:  model.Binding,
		Decision: domain.DecisionSpec{Kind: decisionKind},
		Source:   domain.SourceRef{},
		Payload:  encoded,
	}, nil
}

// RuntimeSpecFromModel decodes the draft model definition into the runtime execution spec.
func RuntimeSpecFromModel(model *domain.AssessmentModel) (*modeltypology.RuntimeSpec, error) {
	_, runtime, err := PayloadAndRuntimeSpecFromModel(model)
	return runtime, err
}

// PayloadAndRuntimeSpecFromModel decodes the draft model definition and preserves payload-level metadata.
func PayloadAndRuntimeSpecFromModel(model *domain.AssessmentModel) (*modeltypology.Payload, *modeltypology.RuntimeSpec, error) {
	if model == nil {
		return nil, nil, fmt.Errorf("assessment model is nil")
	}
	var payload modeltypology.Payload
	if err := json.Unmarshal(model.Definition.Data, &payload); err == nil && (payload.HasExplicitRuntime() || payload.Algorithm != "" || len(payload.Dimensions) > 0) {
		if payload.Algorithm == "" {
			payload.Algorithm = model.Algorithm
		}
		runtime, err := payload.ToRuntimeSpec()
		if err != nil {
			return nil, nil, err
		}
		return &payload, runtime, nil
	}
	var runtime modeltypology.RuntimeSpec
	if err := json.Unmarshal(model.Definition.Data, &runtime); err != nil {
		return nil, nil, fmt.Errorf("decode personality runtime spec: %w", err)
	}
	wrapped := &modeltypology.Payload{
		Algorithm: model.Algorithm,
		Runtime:   &runtime,
	}
	resolved, err := wrapped.ToRuntimeSpec()
	if err != nil {
		return nil, nil, err
	}
	return wrapped, resolved, nil
}

func preparePublishedPayload(payload *modeltypology.Payload, model *domain.AssessmentModel, runtime *modeltypology.RuntimeSpec) {
	payload.Code = model.Code
	payload.Version = modelVersionString(model)
	payload.Title = model.Title
	payload.QuestionnaireCode = model.Binding.QuestionnaireCode
	payload.QuestionnaireVersion = model.Binding.QuestionnaireVersion
	payload.Status = string(domain.ModelStatusPublished)
	payload.Algorithm = model.Algorithm
	payload.Runtime = runtime
}

func modelVersionString(model *domain.AssessmentModel) string {
	return "v" + strconv.FormatInt(model.Version, 10)
}
