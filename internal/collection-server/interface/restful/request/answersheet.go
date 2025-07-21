package request

import "time"

// AnswerValue 答案值
type AnswerValue struct {
	QuestionCode string      `json:"question_code" binding:"required"`
	QuestionType string      `json:"question_type" binding:"required"`
	Value        interface{} `json:"value" binding:"required"`
}

// AnswersheetSubmitRequest 提交答卷请求
type AnswersheetSubmitRequest struct {
	QuestionnaireCode string        `json:"questionnaire_code" binding:"required,min=3,max=50"`
	TesteeInfo        TesteeInfo    `json:"testee_info" binding:"required"`
	Answers           []AnswerValue `json:"answers" binding:"required,min=1"`
	SubmissionTime    *time.Time    `json:"submission_time,omitempty"`
	DeviceInfo        *DeviceInfo   `json:"device_info,omitempty"`
}

// TesteeInfo 受测者信息
type TesteeInfo struct {
	Name      string                 `json:"name" binding:"required,min=1,max=100"`
	Gender    string                 `json:"gender" binding:"omitempty,oneof=male female other unknown '' 男 女 未知"`
	Age       *int                   `json:"age" binding:"omitempty,min=1,max=120"`
	Phone     string                 `json:"phone" binding:"omitempty"`
	Email     string                 `json:"email" binding:"omitempty,email"`
	ID        string                 `json:"id" binding:"omitempty"`
	ExtraInfo map[string]interface{} `json:"extra_info,omitempty"`
}

// DeviceInfo 设备信息
type DeviceInfo struct {
	DeviceType string `json:"device_type" binding:"omitempty"`
	OS         string `json:"os" binding:"omitempty"`
	Browser    string `json:"browser" binding:"omitempty"`
	UserAgent  string `json:"user_agent" binding:"omitempty"`
	IP         string `json:"ip" binding:"omitempty"`
}

// AnswersheetGetRequest 获取答卷请求
type AnswersheetGetRequest struct {
	ID string `uri:"id" binding:"required" json:"id"`
}

// AnswersheetListRequest 获取答卷列表请求
type AnswersheetListRequest struct {
	Page              int    `form:"page,default=1" json:"page"`
	PageSize          int    `form:"page_size,default=10" json:"page_size"`
	QuestionnaireCode string `form:"questionnaire_code" json:"questionnaire_code"`
	TesteeID          string `form:"testee_id" json:"testee_id"`
	DateFrom          string `form:"date_from" json:"date_from"`
	DateTo            string `form:"date_to" json:"date_to"`
}

// AnswersheetValidateRequest 验证答卷请求
type AnswersheetValidateRequest struct {
	QuestionnaireCode string        `json:"questionnaire_code" binding:"required,min=3,max=50"`
	Answers           []AnswerValue `json:"answers" binding:"required,min=1"`
}

// AnswersheetDeleteRequest 删除答卷请求
type AnswersheetDeleteRequest struct {
	ID string `uri:"id" binding:"required" json:"id"`
}

// AnswersheetBatchDeleteRequest 批量删除答卷请求
type AnswersheetBatchDeleteRequest struct {
	IDs []string `json:"ids" binding:"required,min=1"`
}

// AnswersheetExportRequest 导出答卷请求
type AnswersheetExportRequest struct {
	QuestionnaireCode string `form:"questionnaire_code" binding:"required"`
	Format            string `form:"format,default=xlsx" binding:"oneof=xlsx csv"`
	DateFrom          string `form:"date_from" json:"date_from"`
	DateTo            string `form:"date_to" json:"date_to"`
}
