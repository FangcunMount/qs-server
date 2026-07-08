package norming

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

const definitionNormingExtensionKey = "brief2"

type definitionNormingExtension struct {
	PrimaryDimensionCode string `json:"primary_dimension_code,omitempty"`
}

// HasDefinitionExtension reports whether draft definition payload carries norming metadata.
func HasDefinitionExtension(payload []byte) bool {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(payload, &body); err != nil {
		return false
	}
	_, ok := body[definitionNormingExtensionKey]
	return ok
}

// RequirePrimaryDimensionCodeForPublish validates norming publish metadata when extension exists.
func RequirePrimaryDimensionCodeForPublish(payload []byte) ([]byte, error) {
	if !HasDefinitionExtension(payload) {
		return payload, nil
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("decode norming definition payload: %w", err)
	}
	extension := definitionNormingExtension{}
	if raw, ok := body[definitionNormingExtensionKey]; ok {
		if err := json.Unmarshal(raw, &extension); err != nil {
			return nil, fmt.Errorf("decode norming definition extension: %w", err)
		}
	}
	if extension.PrimaryDimensionCode == "" {
		return nil, fmt.Errorf("norming primary_dimension_code is required for publish")
	}
	return payload, nil
}

// DecisionKindFromDefinitionPayload derives publish decision from norming extension presence.
func DecisionKindFromDefinitionPayload(payload []byte) binding.DecisionKind {
	if HasDefinitionExtension(payload) {
		return binding.DecisionKindNormLookup
	}
	return binding.DecisionKindScoreRange
}
