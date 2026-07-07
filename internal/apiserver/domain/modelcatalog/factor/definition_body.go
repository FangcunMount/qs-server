package factor

import "encoding/json"

// DefinitionBody 是共享 draft/published JSON body 用于 计分 载荷。
type DefinitionBody struct {
	Dimensions     []DimensionRule `json:"dimensions"`
	InterpretRules []InterpretRule `json:"interpret_rules"`
}

// ParseDefinitionBodyJSON de编码 共享 维度 + interpret_rules 载荷 结构。
func ParseDefinitionBodyJSON(payload []byte) (DefinitionBody, error) {
	var body DefinitionBody
	if err := json.Unmarshal(payload, &body); err != nil {
		return DefinitionBody{}, err
	}
	return body, nil
}

// MarshalDefinitionBodyJSON en编码 共享 维度 + interpret_rules 载荷 结构。
func MarshalDefinitionBodyJSON(body DefinitionBody) ([]byte, error) {
	return json.Marshal(body)
}

// FactorsFromDefinitionBodyJSON de编码 和 materializes 规范 因子 in 一个step。
func FactorsFromDefinitionBodyJSON(payload []byte) ([]FactorSnapshot, error) {
	body, err := ParseDefinitionBodyJSON(payload)
	if err != nil {
		return nil, err
	}
	return ParseFactorsFromDefinitionBody(body.Dimensions, body.InterpretRules), nil
}
