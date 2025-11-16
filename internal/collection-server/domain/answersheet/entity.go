package answersheet

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Answersheet 答卷实体
type Answersheet struct {
	ID                 meta.ID     `json:"id"`
	QuestionnaireCode  string      `json:"questionnaire_code"`
	QuestionnaireTitle string      `json:"questionnaire_title"`
	Title              string      `json:"title"`
	WriterID           meta.ID     `json:"writer_id"`
	TesteeID           meta.ID     `json:"testee_id"`
	TesteeInfo         *TesteeInfo `json:"testee_info"`
	Answers            []*Answer   `json:"answers"`
	Status             string      `json:"status"`
	SubmittedAt        time.Time   `json:"submitted_at"`
	CreatedAt          time.Time   `json:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at"`
}

// Answer 答案实体
type Answer struct {
	ID           meta.ID     `json:"id"`
	QuestionCode string      `json:"question_code"`
	QuestionType string      `json:"question_type"`
	Value        interface{} `json:"value"`
	Score        float64     `json:"score,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
}

// TesteeInfo 测试者信息
type TesteeInfo struct {
	Name   string `json:"name"`
	Age    int    `json:"age"`
	Gender string `json:"gender"`
	Email  string `json:"email,omitempty"`
	Phone  string `json:"phone,omitempty"`
}

// SubmitRequest 提交答卷请求
type SubmitRequest struct {
	QuestionnaireCode string      `json:"questionnaire_code"`
	Title             string      `json:"title"`
	WriterID          meta.ID     `json:"writer_id"`
	TesteeID          meta.ID     `json:"testee_id"`
	TesteeInfo        *TesteeInfo `json:"testee_info"`
	Answers           []*Answer   `json:"answers"`
}

// SubmitResponse 提交答卷响应
type SubmitResponse struct {
	ID        meta.ID   `json:"id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// ValidationRequest 验证请求
type ValidationRequest struct {
	QuestionnaireCode string      `json:"questionnaire_code"`
	Answers           []*Answer   `json:"answers"`
	TesteeInfo        *TesteeInfo `json:"testee_info"`
}

// ValidationError 验证错误
type ValidationError struct {
	QuestionCode string      `json:"question_code"`
	Field        string      `json:"field"`
	Message      string      `json:"message"`
	Value        interface{} `json:"value,omitempty"`
}
