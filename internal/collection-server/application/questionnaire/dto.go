package questionnaire

// QuestionnaireResponse 问卷响应
type QuestionnaireResponse struct {
	Code        string             `json:"code"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	ImgURL      string             `json:"img_url"`
	Status      string             `json:"status"`
	Version     string             `json:"version"`
	Questions   []QuestionResponse `json:"questions"`
	CreatedAt   string             `json:"created_at"`
	UpdatedAt   string             `json:"updated_at"`
}

// QuestionResponse 问题响应
type QuestionResponse struct {
	Code            string                   `json:"code"`
	Type            string                   `json:"type"`
	Title           string                   `json:"title"`
	Tips            string                   `json:"tips,omitempty"`
	Placeholder     string                   `json:"placeholder,omitempty"`
	Options         []OptionResponse         `json:"options,omitempty"`
	ValidationRules []ValidationRuleResponse `json:"validation_rules,omitempty"`
	CalculationRule *CalculationRuleResponse `json:"calculation_rule,omitempty"`
}

// OptionResponse 选项响应
type OptionResponse struct {
	Code    string `json:"code"`
	Content string `json:"content"`
	Score   int32  `json:"score"`
}

// ValidationRuleResponse 验证规则响应
type ValidationRuleResponse struct {
	RuleType    string `json:"rule_type"`
	TargetValue string `json:"target_value"`
}

// CalculationRuleResponse 计算规则响应
type CalculationRuleResponse struct {
	FormulaType string `json:"formula_type"`
}

// ListQuestionnairesRequest 问卷列表请求
type ListQuestionnairesRequest struct {
	Page     int32  `form:"page"`
	PageSize int32  `form:"page_size"`
	Status   string `form:"status"`
	Title    string `form:"title"`
}

// ListQuestionnairesResponse 问卷列表响应
type ListQuestionnairesResponse struct {
	Questionnaires []QuestionnaireResponse `json:"questionnaires"`
	Total          int64                   `json:"total"`
	Page           int32                   `json:"page"`
	PageSize       int32                   `json:"page_size"`
}
