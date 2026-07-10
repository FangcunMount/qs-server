package behavioral

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

const brief2ExtensionKey = "brief2"

type brief2PublishExtension struct {
	PrimaryDimensionCode string `json:"primary_dimension_code,omitempty"`
}

// PrepareDefinitionForPublish validates BRIEF-2 metadata while preserving the input bytes.
func PrepareDefinitionForPublish(payload []byte) ([]byte, error) {
	if !hasBrief2Extension(payload) {
		return payload, nil
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("decode norming definition payload: %w", err)
	}
	extension := brief2PublishExtension{}
	if raw, ok := body[brief2ExtensionKey]; ok {
		if err := json.Unmarshal(raw, &extension); err != nil {
			return nil, fmt.Errorf("decode norming definition extension: %w", err)
		}
	}
	if extension.PrimaryDimensionCode == "" {
		return nil, fmt.Errorf("norming primary_dimension_code is required for publish")
	}
	return payload, nil
}

// DecisionKindFromDefinitionPayload derives the publish decision from BRIEF-2 metadata.
func DecisionKindFromDefinitionPayload(payload []byte) binding.DecisionKind {
	if hasBrief2Extension(payload) {
		return binding.DecisionKindNormLookup
	}
	return binding.DecisionKindScoreRange
}

func hasBrief2Extension(payload []byte) bool {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(payload, &body); err != nil {
		return false
	}
	_, ok := body[brief2ExtensionKey]
	return ok
}
