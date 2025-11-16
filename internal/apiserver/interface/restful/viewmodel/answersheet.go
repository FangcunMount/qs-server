package viewmodel

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// SaveAnswerSheetRequest 保存答卷请求视图模型
type SaveAnswerSheetRequest struct {
	QuestionnaireCode    string      `json:"questionnaire_code" valid:"required"`
	QuestionnaireVersion string      `json:"questionnaire_version" valid:"required"`
	Title                string      `json:"title" valid:"required"`
	WriterID             uint64      `json:"writer_id" valid:"required"`
	TesteeID             uint64      `json:"testee_id" valid:"required"`
	Answers              []AnswerDTO `json:"answers" valid:"required"`
}

// ListAnswerSheetsRequest 获取答卷列表请求视图模型
type ListAnswerSheetsRequest struct {
	QuestionnaireCode    string `form:"questionnaire_code"`
	QuestionnaireVersion string `form:"questionnaire_version"`
	WriterID             uint64 `form:"writer_id"`
	TesteeID             uint64 `form:"testee_id"`
	Page                 int    `form:"page" binding:"required,min=1"`
	PageSize             int    `form:"page_size" binding:"required,min=1,max=100"`
}

// AnswerSheetViewModel 答卷视图模型
type AnswerSheetViewModel struct {
	ID                   meta.ID     `json:"id"`
	QuestionnaireCode    string      `json:"questionnaire_code"`
	QuestionnaireVersion string      `json:"questionnaire_version"`
	Title                string      `json:"title"`
	Score                float64     `json:"score"`
	WriterID             uint64      `json:"writer_id"`
	TesteeID             uint64      `json:"testee_id"`
	Answers              []AnswerDTO `json:"answers"`
}

// AnswerSheetDetailViewModel 答卷详情视图模型
type AnswerSheetDetailViewModel struct {
	AnswerSheet   AnswerSheetViewModel `json:"answer_sheet"`
	WriterName    string               `json:"writer_name"`
	TesteeName    string               `json:"testee_name"`
	Questionnaire dto.QuestionnaireDTO `json:"questionnaire"`
	CreatedAt     string               `json:"created_at"`
	UpdatedAt     string               `json:"updated_at"`
}
