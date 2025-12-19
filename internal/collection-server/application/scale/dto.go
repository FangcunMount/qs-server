package scale

// ScaleResponse 量表响应
type ScaleResponse struct {
	Code                 string           `json:"code"`
	Title                string           `json:"title"`
	Description          string           `json:"description"`
	Category             string           `json:"category"`
	Stages               []string         `json:"stages"`
	ApplicableAges       []string         `json:"applicable_ages"`
	Reporters            []string         `json:"reporters"`
	Tags                 []string         `json:"tags"`
	QuestionnaireCode    string           `json:"questionnaire_code"`
	QuestionnaireVersion string           `json:"questionnaire_version"`
	Status               string           `json:"status"`
	Factors              []FactorResponse `json:"factors"`
}

// FactorResponse 因子响应
type FactorResponse struct {
	Code            string                  `json:"code"`
	Title           string                  `json:"title"`
	FactorType      string                  `json:"factor_type"`
	IsTotalScore    bool                    `json:"is_total_score"`
	QuestionCodes   []string                `json:"question_codes"`
	ScoringStrategy string                  `json:"scoring_strategy"`
	ScoringParams   map[string]string       `json:"scoring_params"`
	RiskLevel       string                  `json:"risk_level"`
	InterpretRules  []InterpretRuleResponse `json:"interpret_rules"`
}

// InterpretRuleResponse 解读规则响应
type InterpretRuleResponse struct {
	MinScore   float64 `json:"min_score"`
	MaxScore   float64 `json:"max_score"`
	RiskLevel  string  `json:"risk_level"`
	Conclusion string  `json:"conclusion"`
	Suggestion string  `json:"suggestion"`
}

// ScaleSummaryResponse 量表摘要响应（列表查询，不含因子详情）
type ScaleSummaryResponse struct {
	Code                 string   `json:"code"`
	Title                string   `json:"title"`
	Description          string   `json:"description"`
	Category             string   `json:"category"`
	Stages               []string `json:"stages"`
	ApplicableAges       []string `json:"applicable_ages"`
	Reporters            []string `json:"reporters"`
	Tags                 []string `json:"tags"`
	QuestionnaireCode    string   `json:"questionnaire_code"`
	QuestionnaireVersion string   `json:"questionnaire_version"`
	Status               string   `json:"status"`
}

// ListScalesRequest 量表列表请求
type ListScalesRequest struct {
	Page           int32    `form:"page"`
	PageSize       int32    `form:"page_size"`
	Status         string   `form:"status"`
	Title          string   `form:"title"`
	Category       string   `form:"category"`
	Stages         []string `form:"stages"`
	ApplicableAges []string `form:"applicable_ages"`
	Reporters      []string `form:"reporters"`
	Tags           []string `form:"tags"`
}

// ListScalesResponse 量表列表响应
type ListScalesResponse struct {
	Scales    []ScaleSummaryResponse `json:"scales"`
	Total     int64                  `json:"total"`
	Page      int32                  `json:"page"`
	PageSize  int32                  `json:"page_size"`
}

// ScaleCategoriesResponse 量表分类响应
type ScaleCategoriesResponse struct {
	Categories     []CategoryResponse     `json:"categories"`
	Stages         []StageResponse         `json:"stages"`
	ApplicableAges []ApplicableAgeResponse `json:"applicable_ages"`
	Reporters      []ReporterResponse      `json:"reporters"`
	Tags           []TagResponse           `json:"tags"`
}

// CategoryResponse 类别响应
type CategoryResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// StageResponse 阶段响应
type StageResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ApplicableAgeResponse 使用年龄响应
type ApplicableAgeResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ReporterResponse 填报人响应
type ReporterResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// TagResponse 标签响应
type TagResponse struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Category string `json:"category"`
}

