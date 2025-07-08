package request

// CreateMedicalScaleRequest 创建医学量表请求
type CreateMedicalScaleRequest struct {
	Code                 string `json:"code" binding:"required"`
	Title                string `json:"title" binding:"required"`
	QuestionnaireCode    string `json:"questionnaire_code" binding:"required"`
	QuestionnaireVersion string `json:"questionnaire_version" binding:"required"`
}

// UpdateMedicalScaleRequest 更新医学量表基础信息请求
type UpdateMedicalScaleRequest struct {
	Title                string `json:"title" binding:"required"`
	QuestionnaireCode    string `json:"questionnaire_code" binding:"required"`
	QuestionnaireVersion string `json:"questionnaire_version" binding:"required"`
}

// UpdateMedicalScaleFactorRequest 更新医学量表因子请求
type UpdateMedicalScaleFactorRequest struct {
	Code    string      `json:"code" binding:"required"`
	Factors []FactorDTO `json:"factors" binding:"required,min=1"`
}

// FactorDTO 因子请求
type FactorDTO struct {
	Code            string                 `json:"code" binding:"required"`
	Title           string                 `json:"title" binding:"required"`
	IsTotalScore    bool                   `json:"is_total_score"`
	FactorType      string                 `json:"factor_type" binding:"required"`
	CalculationRule CalculationRuleRequest `json:"calculation_rule" binding:"required"`
	InterpretRules  []InterpretRuleRequest `json:"interpret_rules"`
}

// CalculationRuleRequest 计算规则请求
type CalculationRuleRequest struct {
	FormulaType string   `json:"formula_type" binding:"required"`
	SourceCodes []string `json:"source_codes" binding:"required,min=1"`
}

// InterpretRuleRequest 解读规则请求
type InterpretRuleRequest struct {
	ScoreRange ScoreRangeRequest `json:"score_range" binding:"required"`
	Content    string            `json:"content" binding:"required"`
}

// ScoreRangeRequest 分数范围请求
// 分数区间采用左开右闭原则 (min, max]
// 例如：区间 (0,6] 表示分数大于0且小于等于6
// 相邻区间的边界允许相等，例如：(0,6],(6,9] 是合法的
type ScoreRangeRequest struct {
	MinScore float64 `json:"min_score" binding:"required"`
	MaxScore float64 `json:"max_score" binding:"required"`
}
