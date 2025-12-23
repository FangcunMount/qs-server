package request

// ============= Scale Lifecycle Requests =============

// CreateScaleRequest 创建量表请求
type CreateScaleRequest struct {
	Title                string   `json:"title" valid:"required~量表标题不能为空"`
	Description          string   `json:"description"`
	Category             string   `json:"category"`
	Stages               []string `json:"stages"`
	ApplicableAges       []string `json:"applicable_ages"`
	Reporters            []string `json:"reporters"`
	Tags                 []string `json:"tags"`
	QuestionnaireCode    string   `json:"questionnaire_code"`
	QuestionnaireVersion string   `json:"questionnaire_version"`
}

// UpdateScaleBasicInfoRequest 更新量表基本信息请求
type UpdateScaleBasicInfoRequest struct {
	Title          string   `json:"title" valid:"required~量表标题不能为空"`
	Description    string   `json:"description"`
	Category       string   `json:"category"`
	Stages         []string `json:"stages"`
	ApplicableAges []string `json:"applicable_ages"`
	Reporters      []string `json:"reporters"`
	Tags           []string `json:"tags"`
}

// UpdateScaleQuestionnaireRequest 更新量表关联问卷请求
type UpdateScaleQuestionnaireRequest struct {
	QuestionnaireCode    string `json:"questionnaire_code" valid:"required~问卷编码不能为空"`
	QuestionnaireVersion string `json:"questionnaire_version" valid:"required~问卷版本不能为空"`
}

// ============= Scale Factor Requests =============

// BatchUpdateFactorsRequest 批量更新因子请求
type BatchUpdateFactorsRequest struct {
	Factors []FactorModel `json:"factors" valid:"required~因子列表不能为空"`
}

// ReplaceInterpretRulesRequest 批量设置解读规则请求
type ReplaceInterpretRulesRequest struct {
	FactorRules []FactorInterpretRulesModel `json:"factor_rules" valid:"required~因子解读规则列表不能为空"`
}

// FactorInterpretRulesModel 因子解读规则模型
type FactorInterpretRulesModel struct {
	FactorCode     string               `json:"factor_code" valid:"required~因子编码不能为空"`
	InterpretRules []InterpretRuleModel `json:"interpret_rules"`
}

// ============= Shared Models =============

// FactorModel 因子模型（用于请求）
type FactorModel struct {
	Code            string               `json:"code" valid:"required~因子编码不能为空"`
	Title           string               `json:"title" valid:"required~因子标题不能为空"`
	FactorType      string               `json:"factor_type"`
	IsTotalScore    bool                 `json:"is_total_score"`
	IsShow          bool                 `json:"is_show"` // 是否显示（用于报告中的维度展示）
	QuestionCodes   []string             `json:"question_codes"`
	ScoringStrategy string               `json:"scoring_strategy"`
	ScoringParams   *ScoringParamsModel  `json:"scoring_params,omitempty"`
	MaxScore        *float64             `json:"max_score,omitempty"` // 最大分
	RiskLevel       string               `json:"risk_level,omitempty"` // 因子级别的风险等级（用于批量设置，如果解读规则未指定则使用此值）
	InterpretRules  []InterpretRuleModel `json:"interpret_rules"`
}

// ScoringParamsModel 计分参数模型
// 根据不同的计分策略，使用不同的字段
type ScoringParamsModel struct {
	// 计数策略（cnt）专用参数
	CntOptionContents []string `json:"cnt_option_contents,omitempty"`

	// 其他策略的扩展参数（可选）
	// 如果需要添加其他策略的参数，可以在这里扩展
	// CustomParams map[string]interface{} `json:"custom_params,omitempty"`
}

// InterpretRuleModel 解读规则模型（用于请求）
type InterpretRuleModel struct {
	MinScore   float64 `json:"min_score"`
	MaxScore   float64 `json:"max_score"`
	RiskLevel  string  `json:"risk_level"`
	Conclusion string  `json:"conclusion"`
	Suggestion string  `json:"suggestion"`
}

// ============= Query Requests =============

// ListScalesRequest 查询量表列表请求
type ListScalesRequest struct {
	Page       int               `form:"page" valid:"required~页码不能为空"`
	PageSize   int               `form:"page_size" valid:"required~每页数量不能为空"`
	Conditions map[string]string `form:"conditions"`
}
