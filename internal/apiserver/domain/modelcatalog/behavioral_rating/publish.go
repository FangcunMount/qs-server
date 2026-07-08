package behavioral_rating

import (
	"encoding/json"
	"fmt"
	"strconv"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// BuildPublishedSnapshot 物化v2 已发布快照 从 draft behavioral_rating model。
func BuildPublishedSnapshot(model *domain.AssessmentModel) (*domain.PublishedModelSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	if model.Kind != domain.KindBehavioralRating {
		return nil, fmt.Errorf("model kind %s is not behavioral_rating", model.Kind)
	}
	if model.Definition.IsEmpty() {
		return nil, fmt.Errorf("behavioral_rating model definition is empty")
	}
	encoded := append([]byte(nil), model.Definition.Data...)
	if !json.Valid(encoded) {
		return nil, fmt.Errorf("behavioral_rating model definition is not valid json")
	}
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmBrief2
	}
	encoded, err := requireNormingPrimaryDimensionCode(encoded)
	if err != nil {
		return nil, err
	}
	return &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatForBehavioralRating(algorithm),
		Model: domain.ModelDefinition{
			ProductChannel: domain.ResolveProductChannel(model.Kind, model.ProductChannel),
			Kind:           domain.KindBehavioralRating,
			SubKind:        domain.SubKindEmpty,
			Algorithm:      algorithm,
			Code:           model.Code,
			Version:        modelVersionString(model),
			Title:          model.Title,
			Status:         string(domain.ModelStatusPublished),
		},
		Binding:  model.Binding,
		Decision: normingDecisionSpecFromPayload(encoded),
		Source:   domain.SourceRef{},
		Payload:  encoded,
	}, nil
}

func modelVersionString(model *domain.AssessmentModel) string {
	return "v" + strconv.FormatInt(model.Version, 10)
}

func normingDecisionSpecFromPayload(payload []byte) domain.DecisionSpec {
	if hasNormingExtension(payload) {
		return domain.DecisionSpec{Kind: domain.DecisionKindNormLookup}
	}
	return domain.DecisionSpec{Kind: domain.DecisionKindScoreRange}
}

func hasNormingExtension(payload []byte) bool {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(payload, &body); err != nil {
		return false
	}
	_, ok := body["brief2"]
	return ok
}

func requireNormingPrimaryDimensionCode(payload []byte) ([]byte, error) {
	if !hasNormingExtension(payload) {
		return payload, nil
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("decode behavioral_rating norming payload: %w", err)
	}
	brief2 := map[string]json.RawMessage{}
	if raw, ok := body["brief2"]; ok {
		if err := json.Unmarshal(raw, &brief2); err != nil {
			return nil, fmt.Errorf("decode behavioral_rating norming extension: %w", err)
		}
	}
	var primaryDimensionCode string
	if raw, ok := brief2["primary_dimension_code"]; ok {
		if err := json.Unmarshal(raw, &primaryDimensionCode); err != nil {
			return nil, fmt.Errorf("decode behavioral_rating primary_dimension_code: %w", err)
		}
	}
	if primaryDimensionCode == "" {
		return nil, fmt.Errorf("brief2.primary_dimension_code is required for publish")
	}
	return payload, nil
}
