package factor

import "encoding/json"

// DefinitionBody is the shared draft/published JSON body for scoring payloads.
type DefinitionBody struct {
	Dimensions     []DimensionRule `json:"dimensions"`
	InterpretRules []InterpretRule `json:"interpret_rules"`
}

// ParseDefinitionBodyJSON decodes the shared dimensions + interpret_rules payload shape.
func ParseDefinitionBodyJSON(payload []byte) (DefinitionBody, error) {
	var body DefinitionBody
	if err := json.Unmarshal(payload, &body); err != nil {
		return DefinitionBody{}, err
	}
	return body, nil
}

// MarshalDefinitionBodyJSON encodes the shared dimensions + interpret_rules payload shape.
func MarshalDefinitionBodyJSON(body DefinitionBody) ([]byte, error) {
	return json.Marshal(body)
}

// FactorsFromDefinitionBodyJSON decodes and materializes canonical factors in one step.
func FactorsFromDefinitionBodyJSON(payload []byte) ([]FactorSnapshot, error) {
	body, err := ParseDefinitionBodyJSON(payload)
	if err != nil {
		return nil, err
	}
	return ParseFactorsFromDefinitionBody(body.Dimensions, body.InterpretRules), nil
}
