package dto

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// MedicalScaleDTO 医学量表数据传输对象
type MedicalScaleDTO struct {
	ID                meta.ID     `json:"id"`
	Code              string      `json:"code"`
	QuestionnaireCode string      `json:"questionnaire_code"`
	Title             string      `json:"title"`
	Description       string      `json:"description"`
	Factors           []FactorDTO `json:"factors"`
}

// FactorDTO 因子数据传输对象
type FactorDTO struct {
	Code            string              `json:"code"`
	Title           string              `json:"title"`
	FactorType      string              `json:"factor_type"`
	IsTotalScore    bool                `json:"is_total_score"`
	CalculationRule *CalculationRuleDTO `json:"calculation_rule"`
	InterpretRules  []InterpretRuleDTO  `json:"interpret_rules"`
}

// InterpretRuleDTO 解读规则数据传输对象
type InterpretRuleDTO struct {
	ScoreRange ScoreRangeDTO `json:"score_range"`
	Content    string        `json:"content"`
}

// ScoreRangeDTO 分数范围
type ScoreRangeDTO struct {
	MinScore float64 `json:"min_score"`
	MaxScore float64 `json:"max_score"`
}
