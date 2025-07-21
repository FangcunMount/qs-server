package response

import "time"

// ErrorResponse 错误响应
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Answer 答案信息
type Answer struct {
	QuestionCode  string      `json:"question_code"`
	QuestionTitle string      `json:"question_title,omitempty"`
	QuestionType  string      `json:"question_type,omitempty"`
	Value         interface{} `json:"value"`
	DisplayValue  string      `json:"display_value,omitempty"`
	Points        *int        `json:"points,omitempty"`
}

// TesteeResponseInfo 受测者响应信息
type TesteeResponseInfo struct {
	Name      string                 `json:"name"`
	Gender    string                 `json:"gender,omitempty"`
	Age       *int                   `json:"age,omitempty"`
	Phone     string                 `json:"phone,omitempty"`
	Email     string                 `json:"email,omitempty"`
	ID        string                 `json:"id,omitempty"`
	ExtraInfo map[string]interface{} `json:"extra_info,omitempty"`
}

// DeviceResponseInfo 设备响应信息
type DeviceResponseInfo struct {
	DeviceType string `json:"device_type,omitempty"`
	OS         string `json:"os,omitempty"`
	Browser    string `json:"browser,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`
	IP         string `json:"ip,omitempty"`
}

// AnswersheetResponse 答卷响应
type AnswersheetResponse struct {
	ID                 string              `json:"id"`
	QuestionnaireCode  string              `json:"questionnaire_code"`
	QuestionnaireTitle string              `json:"questionnaire_title,omitempty"`
	TesteeInfo         TesteeResponseInfo  `json:"testee_info"`
	Answers            []Answer            `json:"answers"`
	SubmissionTime     time.Time           `json:"submission_time"`
	Status             string              `json:"status"`
	TotalScore         *float64            `json:"total_score,omitempty"`
	CompletionTime     *float64            `json:"completion_time,omitempty"`
	DeviceInfo         *DeviceResponseInfo `json:"device_info,omitempty"`
	ValidationStatus   string              `json:"validation_status"`
	ValidationErrors   []ValidationError   `json:"validation_errors,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

// ValidationError 验证错误
type ValidationError struct {
	QuestionCode string `json:"question_code"`
	Field        string `json:"field"`
	ErrorType    string `json:"error_type"`
	Message      string `json:"message"`
}

// AnswersheetSubmitResponse 提交答卷响应
type AnswersheetSubmitResponse struct {
	ID                string            `json:"id"`
	QuestionnaireCode string            `json:"questionnaire_code"`
	Status            string            `json:"status"`
	SubmissionTime    time.Time         `json:"submission_time"`
	ValidationStatus  string            `json:"validation_status"`
	ValidationErrors  []ValidationError `json:"validation_errors,omitempty"`
	TotalScore        *float64          `json:"total_score,omitempty"`
	NextSteps         []NextStep        `json:"next_steps,omitempty"`
	Message           string            `json:"message"`
}

// NextStep 下一步操作
type NextStep struct {
	Type        string                 `json:"type"` // "redirect", "message", "evaluation"
	Description string                 `json:"description"`
	URL         string                 `json:"url,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// AnswersheetListResponse 答卷列表响应
type AnswersheetListResponse struct {
	Total        int64             `json:"total"`
	Page         int               `json:"page"`
	PageSize     int               `json:"page_size"`
	TotalPages   int               `json:"total_pages"`
	Answersheets []AnswersheetItem `json:"answersheets"`
}

// AnswersheetItem 答卷列表项
type AnswersheetItem struct {
	ID                 string    `json:"id"`
	QuestionnaireCode  string    `json:"questionnaire_code"`
	QuestionnaireTitle string    `json:"questionnaire_title,omitempty"`
	TesteeName         string    `json:"testee_name"`
	SubmissionTime     time.Time `json:"submission_time"`
	Status             string    `json:"status"`
	ValidationStatus   string    `json:"validation_status"`
}
