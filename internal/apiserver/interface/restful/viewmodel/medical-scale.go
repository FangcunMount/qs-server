package viewmodel

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// MedicalScaleVM 医学量表视图模型
type MedicalScaleVM struct {
	ID                meta.ID    `json:"id"`
	Code              string     `json:"code"`
	Title             string     `json:"title"`
	Description       string     `json:"description"`
	QuestionnaireCode string     `json:"questionnaire_code"`
	Version           string     `json:"questionnaire_version"`
	Factors           []FactorVM `json:"factors"`
}

// FactorVM 因子视图模型
type FactorVM struct {
	Code            string            `json:"code"`
	Title           string            `json:"title"`
	IsTotalScore    bool              `json:"is_total_score"`
	FactorType      string            `json:"factor_type"`
	CalculationRule CalculationRuleVM `json:"calculation_rule"`
	InterpretRules  []InterpretRuleVM `json:"interpret_rules"`
}

// CalculationRuleVM 计算规则视图模型
type CalculationRuleVM struct {
	FormulaType string   `json:"formula_type"`
	SourceCodes []string `json:"source_codes"`
}

// InterpretRuleVM 解读规则视图模型
type InterpretRuleVM struct {
	ScoreRange ScoreRangeVM `json:"score_range"`
	Content    string       `json:"content"`
}

// ScoreRangeVM 分数范围视图模型
type ScoreRangeVM struct {
	MinScore float64 `json:"min_score"`
	MaxScore float64 `json:"max_score"`
}
