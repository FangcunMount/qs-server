package assessmentmodel

import (
	"encoding/json"
	"fmt"
	"strconv"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/codec"
)

func BuildPersonalityPublishedSnapshot(model *domain.AssessmentModel) (*domain.PublishedModelSnapshot, error) {
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

	runtime, err := decodePersonalityRuntimeSpec(model)
	if err != nil {
		return nil, err
	}
	payload := &modeltypology.Payload{
		Code:                 model.Code,
		Version:              modelVersionString(model),
		Title:                model.Title,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status:               string(domain.ModelStatusPublished),
		Algorithm:            model.Algorithm,
		Runtime:              runtime,
	}
	encoded, format, err := codec.EncodeTypology(payload)
	if err != nil {
		return nil, err
	}
	decisionKind := runtime.Decision.Kind
	if decisionKind == "" {
		decisionKind = defaultDecisionKind(model.Algorithm)
	}
	return &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: format,
		Model: domain.ModelDefinition{
			Kind:      domain.KindPersonality,
			SubKind:   domain.SubKindTypology,
			Algorithm: model.Algorithm,
			Code:      model.Code,
			Version:   modelVersionString(model),
			Title:     model.Title,
			Status:    string(domain.ModelStatusPublished),
		},
		Binding:  model.Binding,
		Decision: domain.DecisionSpec{Kind: decisionKind},
		Source:   domain.SourceRef{},
		Payload:  encoded,
	}, nil
}

func decodePersonalityRuntimeSpec(model *domain.AssessmentModel) (*modeltypology.RuntimeSpec, error) {
	var payload modeltypology.Payload
	if err := json.Unmarshal(model.Definition.Data, &payload); err == nil && (payload.HasExplicitRuntime() || payload.Algorithm != "" || len(payload.Dimensions) > 0) {
		if payload.Algorithm == "" {
			payload.Algorithm = model.Algorithm
		}
		return payload.ToRuntimeSpec()
	}
	var runtime modeltypology.RuntimeSpec
	if err := json.Unmarshal(model.Definition.Data, &runtime); err != nil {
		return nil, fmt.Errorf("decode personality runtime spec: %w", err)
	}
	wrapped := &modeltypology.Payload{
		Algorithm: model.Algorithm,
		Runtime:   &runtime,
	}
	return wrapped.ToRuntimeSpec()
}

func modelVersionString(model *domain.AssessmentModel) string {
	if model.Binding.QuestionnaireVersion != "" {
		return model.Binding.QuestionnaireVersion
	}
	return "v" + strconv.FormatInt(model.Version, 10)
}

func defaultDecisionKind(algorithm domain.Algorithm) domain.DecisionKind {
	switch algorithm {
	case domain.AlgorithmMBTI:
		return domain.DecisionKindPoleComposition
	case domain.AlgorithmSBTI:
		return domain.DecisionKindNearestPattern
	case domain.AlgorithmBigFive:
		return domain.DecisionKindTraitProfile
	default:
		return domain.DecisionKindScoreRange
	}
}
