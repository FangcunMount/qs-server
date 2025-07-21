package response

import "time"

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
	TotalScore         *float64  `json:"total_score,omitempty"`
	AnswerCount        int       `json:"answer_count"`
	CreatedAt          time.Time `json:"created_at"`
}

// AnswersheetValidateResponse 答卷验证响应
type AnswersheetValidateResponse struct {
	Valid            bool              `json:"valid"`
	ValidationStatus string            `json:"validation_status"`
	ValidationErrors []ValidationError `json:"validation_errors,omitempty"`
	Message          string            `json:"message"`
	Details          ValidationDetails `json:"details,omitempty"`
}

// ValidationDetails 验证详情
type ValidationDetails struct {
	TotalQuestions int            `json:"total_questions"`
	ValidAnswers   int            `json:"valid_answers"`
	InvalidAnswers int            `json:"invalid_answers"`
	MissingAnswers int            `json:"missing_answers"`
	CompletionRate float64        `json:"completion_rate"`
	ErrorsByType   map[string]int `json:"errors_by_type"`
}

// AnswersheetStatsResponse 答卷统计响应
type AnswersheetStatsResponse struct {
	QuestionnaireCode     string       `json:"questionnaire_code"`
	TotalSubmissions      int          `json:"total_submissions"`
	ValidSubmissions      int          `json:"valid_submissions"`
	InvalidSubmissions    int          `json:"invalid_submissions"`
	AverageScore          *float64     `json:"average_score,omitempty"`
	HighestScore          *float64     `json:"highest_score,omitempty"`
	LowestScore           *float64     `json:"lowest_score,omitempty"`
	AverageCompletionTime *float64     `json:"average_completion_time,omitempty"`
	FirstSubmissionTime   *time.Time   `json:"first_submission_time,omitempty"`
	LastSubmissionTime    *time.Time   `json:"last_submission_time,omitempty"`
	SubmissionTrend       []TrendPoint `json:"submission_trend,omitempty"`
}

// 使用response包中的TrendPoint

// AnswersheetOperationResponse 答卷操作响应
type AnswersheetOperationResponse struct {
	Success   bool      `json:"success"`
	Operation string    `json:"operation"`
	Count     int       `json:"count,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

// AnswersheetExportResponse 答卷导出响应
type AnswersheetExportResponse struct {
	ExportID    string    `json:"export_id"`
	Format      string    `json:"format"`
	Status      string    `json:"status"`
	RecordCount int       `json:"record_count"`
	FileSize    int64     `json:"file_size,omitempty"`
	DownloadURL string    `json:"download_url,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	Message     string    `json:"message"`
}
