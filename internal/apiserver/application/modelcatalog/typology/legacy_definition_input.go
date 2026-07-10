package typology

import (
	"encoding/json"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// ImportLegacyDefinitionInput normalizes an API payload and materializes its
// canonical DefinitionV2 before the authoring handler is invoked.
func ImportLegacyDefinitionInput(currentAlgorithm domain.Algorithm, input DefinitionInput) (DefinitionInput, error) {
	format := input.PayloadFormat
	if format == "" {
		format = domain.PayloadFormatPersonalityTypologyV1
	}
	if issues := validateDefinitionPayloadForSave(format, input.Payload); len(issues) > 0 {
		return DefinitionInput{}, validationFailed(issues)
	}
	algorithm := domain.Algorithm(input.Algorithm)
	if currentAlgorithm != "" {
		algorithm = currentAlgorithm
	}
	storedPayload, err := normalizeDefinitionPayloadForStorage(input.Payload, algorithm)
	if err != nil {
		return DefinitionInput{}, err
	}
	definitionV2 := &domain.Definition{}
	if !isEmptyTypologyDraft(storedPayload) {
		materialized, err := modeltypology.ImportLegacyDefinition(storedPayload, algorithm)
		if err != nil {
			return DefinitionInput{}, err
		}
		definitionV2 = materialized.Definition
	}
	input.PayloadFormat = format
	input.Payload = storedPayload
	input.DefinitionV2 = definitionV2
	return input, nil
}

func isEmptyTypologyDraft(payload []byte) bool {
	var body map[string]json.RawMessage
	return json.Unmarshal(payload, &body) == nil && len(body) == 0
}
