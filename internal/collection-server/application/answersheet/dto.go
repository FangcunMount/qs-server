package answersheet

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// SubmitRequest 提交答卷请求
type SubmitRequest struct {
	QuestionnaireCode string      `json:"questionnaire_code" validate:"required"`
	Title             string      `json:"title" validate:"required"`
	TesteeInfo        *TesteeInfo `json:"testee_info" validate:"required"`
	Answers           []*Answer   `json:"answers" validate:"required"`
}

// SubmitResponse 提交答卷响应
type SubmitResponse struct {
	ID        meta.ID   `json:"id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// ValidationRequest 验证答卷请求
type ValidationRequest struct {
	QuestionnaireCode string      `json:"questionnaire_code" validate:"required"`
	Title             string      `json:"title" validate:"required"`
	TesteeInfo        *TesteeInfo `json:"testee_info" validate:"required"`
	Answers           []*Answer   `json:"answers" validate:"required"`
}

// TesteeInfo 测试者信息
type TesteeInfo struct {
	Name   string `json:"name" validate:"required"`
	Gender string `json:"gender,omitempty"`
	Age    *int   `json:"age,omitempty"`
	Email  string `json:"email,omitempty"`
	Phone  string `json:"phone,omitempty"`
}

// Answer 答案
type Answer struct {
	QuestionCode string      `json:"question_code" validate:"required"`
	QuestionType string      `json:"question_type" validate:"required"`
	Value        interface{} `json:"value" validate:"required"`
}

// GetAnswersheetRequest 获取答卷请求
type GetAnswersheetRequest struct {
	ID meta.ID `json:"id" validate:"required"`
}

// GetAnswersheetResponse 获取答卷响应
type GetAnswersheetResponse struct {
	ID                meta.ID     `json:"id"`
	QuestionnaireCode string      `json:"questionnaire_code"`
	Title             string      `json:"title"`
	TesteeInfo        *TesteeInfo `json:"testee_info"`
	Answers           []*Answer   `json:"answers"`
	Status            string      `json:"status"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}
