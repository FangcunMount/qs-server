package response

import "time"

// Question 问题信息
type Question struct {
	Code             string                 `json:"code"`
	Type             string                 `json:"type"`
	Title            string                 `json:"title"`
	Description      string                 `json:"description,omitempty"`
	Required         bool                   `json:"required"`
	Options          []QuestionOption       `json:"options,omitempty"`
	ValidationRules  []ValidationRule       `json:"validation_rules,omitempty"`
	DisplayOrder     int                    `json:"display_order"`
	Group            string                 `json:"group,omitempty"`
	ConditionalLogic map[string]interface{} `json:"conditional_logic,omitempty"`
}

// QuestionOption 问题选项
type QuestionOption struct {
	Code   string `json:"code"`
	Text   string `json:"text"`
	Value  string `json:"value"`
	Order  int    `json:"order"`
	Points int    `json:"points,omitempty"`
}

// ValidationRule 验证规则
type ValidationRule struct {
	RuleType    string      `json:"rule_type"`
	RuleValue   interface{} `json:"rule_value,omitempty"`
	ErrorMsg    string      `json:"error_msg"`
	TargetValue interface{} `json:"target_value,omitempty"`
}

// QuestionnaireResponse 问卷响应
type QuestionnaireResponse struct {
	Code        string     `json:"code"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Category    string     `json:"category,omitempty"`
	Status      string     `json:"status"`
	Version     int        `json:"version"`
	Questions   []Question `json:"questions"`
	Settings    Settings   `json:"settings"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	SubmitCount int        `json:"submit_count"`
	ViewCount   int        `json:"view_count"`
}

// Settings 问卷设置
type Settings struct {
	AllowMultipleSubmissions bool                   `json:"allow_multiple_submissions"`
	RequireLogin             bool                   `json:"require_login"`
	ShowProgressBar          bool                   `json:"show_progress_bar"`
	RandomizeQuestions       bool                   `json:"randomize_questions"`
	TimeLimit                *int                   `json:"time_limit,omitempty"`
	SubmissionMessage        string                 `json:"submission_message,omitempty"`
	RedirectURL              string                 `json:"redirect_url,omitempty"`
	CustomCSS                string                 `json:"custom_css,omitempty"`
	ExtraSettings            map[string]interface{} `json:"extra_settings,omitempty"`
}

// QuestionnaireListResponse 问卷列表响应
type QuestionnaireListResponse struct {
	Total          int64               `json:"total"`
	Page           int                 `json:"page"`
	PageSize       int                 `json:"page_size"`
	TotalPages     int                 `json:"total_pages"`
	Questionnaires []QuestionnaireItem `json:"questionnaires"`
}

// QuestionnaireItem 问卷列表项
type QuestionnaireItem struct {
	Code        string     `json:"code"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Category    string     `json:"category,omitempty"`
	Status      string     `json:"status"`
	Version     int        `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	SubmitCount int        `json:"submit_count"`
	ViewCount   int        `json:"view_count"`
}

// QuestionnaireCreateResponse 创建问卷响应
type QuestionnaireCreateResponse struct {
	Code      string    `json:"code"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Message   string    `json:"message"`
}

// QuestionnaireUpdateResponse 更新问卷响应
type QuestionnaireUpdateResponse struct {
	Code      string    `json:"code"`
	Title     string    `json:"title"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
	Message   string    `json:"message"`
}

// QuestionnaireOperationResponse 问卷操作响应（发布、归档等）
type QuestionnaireOperationResponse struct {
	Code      string    `json:"code"`
	Status    string    `json:"status"`
	Operation string    `json:"operation"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

// QuestionnaireValidateResponse 问卷代码验证响应
type QuestionnaireValidateResponse struct {
	Valid   bool   `json:"valid"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// QuestionnaireStatsResponse 问卷统计响应
type QuestionnaireStatsResponse struct {
	Code               string       `json:"code"`
	Title              string       `json:"title"`
	SubmitCount        int          `json:"submit_count"`
	ViewCount          int          `json:"view_count"`
	CompletionRate     float64      `json:"completion_rate"`
	AverageTimeSpent   float64      `json:"average_time_spent"`
	LastSubmissionTime time.Time    `json:"last_submission_time"`
	TrendData          []TrendPoint `json:"trend_data,omitempty"`
}

// TrendPoint 趋势数据点
type TrendPoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}
