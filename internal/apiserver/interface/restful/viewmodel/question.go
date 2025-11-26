package viewmodel

// QuestionDTO 问题
type QuestionDTO struct {
	Code string `json:"code"`          // 问题ID，仅更新/编辑时提供
	Type string `json:"question_type"` // 问题题型：single_choice, multi_choice, text 等
	Stem string `json:"stem"`          // 问题题干
	Tips string `json:"tips"`          // 问题提示

	// 特定属性
	Placeholder string      `json:"placeholder"`       // 问题占位符
	Options     []OptionDTO `json:"options,omitempty"` // 问题选项（可选项，结构化题型）

	// 能力属性
	ValidationRules []ValidationRuleDTO `json:"validation_rules,omitempty"` // 校验规则（可选项）
	CalculationRule *CalculationRuleDTO `json:"calculation_rule,omitempty"` // 问题算分规则（可选项，结构化题型）
}

// Option 选项
type OptionDTO struct {
	Code    string  `json:"code"`    // 选项ID，仅更新/编辑时提供
	Content string  `json:"content"` // 选项内容
	Score   float64 `json:"score"`   // 选项分数（支持小数）
}

// ValidationRule 校验规则
type ValidationRuleDTO struct {
	RuleType    string `json:"rule_type"`    // 规则类型
	TargetValue string `json:"target_value"` // 目标值
}

// CalculationRule 算分规则
type CalculationRuleDTO struct {
	FormulaType string `json:"formula_type"` // 公式类型
}
