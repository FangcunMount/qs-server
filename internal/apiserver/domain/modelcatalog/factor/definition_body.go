package factor

import "encoding/json"

// DefinitionBody 是共享 draft/published JSON payload adapter。
// 它负责承接历史 payload shape；核心领域语义应物化为 Factor 后再参与校验和推导。
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

// FactorsFromDefinitionBodyJSON de编码 payload 并返回兼容 FactorSnapshot DTO。
func FactorsFromDefinitionBodyJSON(payload []byte) ([]FactorSnapshot, error) {
	body, err := ParseDefinitionBodyJSON(payload)
	if err != nil {
		return nil, err
	}
	return ParseFactorsFromDefinitionBody(body.Dimensions, body.InterpretRules), nil
}

// FactorsFromDefinitionBodyJSONAsFactors decodes shared payload JSON into domain Factors.
func FactorsFromDefinitionBodyJSONAsFactors(payload []byte) ([]Factor, error) {
	snapshots, err := FactorsFromDefinitionBodyJSON(payload)
	if err != nil {
		return nil, err
	}
	return FactorsFromSnapshots(snapshots), nil
}
