package dto

// CalculationRuleDTO 计算规则数据传输对象
type CalculationRuleDTO struct {
	FormulaType string   `json:"formula_type"`
	SourceCodes []string `json:"source_codes"`
}
