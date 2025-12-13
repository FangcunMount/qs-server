package answersheet

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// SubmitAnswerSheetRequest 提交答卷请求
type SubmitAnswerSheetRequest struct {
	QuestionnaireCode    string   `json:"questionnaire_code" binding:"required"`
	QuestionnaireVersion string   `json:"questionnaire_version" binding:"required"`
	Title                string   `json:"title"`
	TesteeID             uint64   `json:"testee_id" binding:"required"`
	Answers              []Answer `json:"answers" binding:"required"`
}

// UnmarshalJSON 自定义 JSON 反序列化，支持 testee_id 为字符串或数字
func (r *SubmitAnswerSheetRequest) UnmarshalJSON(data []byte) error {
	// 使用临时结构体避免递归调用
	type Alias SubmitAnswerSheetRequest
	aux := &struct {
		TesteeID interface{} `json:"testee_id"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// 处理 TesteeID，支持字符串或数字
	switch v := aux.TesteeID.(type) {
	case string:
		// 字符串转数字
		testeeID, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid testee_id format: %w", err)
		}
		r.TesteeID = testeeID
	case float64:
		r.TesteeID = uint64(v)
	default:
		return fmt.Errorf("testee_id must be a string or number")
	}

	return nil
}

// Answer 答案
type Answer struct {
	QuestionCode string `json:"question_code" binding:"required"`
	QuestionType string `json:"question_type" binding:"required"`
	Score        uint32 `json:"score"`
	Value        string `json:"value" binding:"required"` // JSON 字符串
}

// SubmitAnswerSheetResponse 提交答卷响应
type SubmitAnswerSheetResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// GetAnswerSheetRequest 获取答卷请求
type GetAnswerSheetRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}

// AnswerSheetResponse 答卷响应
type AnswerSheetResponse struct {
	ID                   string   `json:"id"`
	QuestionnaireCode    string   `json:"questionnaire_code"`
	QuestionnaireVersion string   `json:"questionnaire_version"`
	Title                string   `json:"title"`
	Score                float64  `json:"score"`
	WriterID             string   `json:"writer_id"`
	WriterName           string   `json:"writer_name"`
	TesteeID             string   `json:"testee_id"`
	TesteeName           string   `json:"testee_name"`
	Answers              []Answer `json:"answers"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}
