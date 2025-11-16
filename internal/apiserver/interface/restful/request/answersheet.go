package request

import "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/viewmodel"

// SaveAnswerSheetRequest 保存答卷请求
type SaveAnswerSheetRequest struct {
	QuestionnaireCode    string                `json:"questionnaire_code" valid:"required"`
	QuestionnaireVersion string                `json:"questionnaire_version" valid:"required"`
	Title                string                `json:"title" valid:"required"`
	WriterID             uint64                `json:"writer_id" valid:"required"`
	TesteeID             uint64                `json:"testee_id" valid:"required"`
	Answers              []viewmodel.AnswerDTO `json:"answers" valid:"required"`
}

// ListAnswerSheetsRequest 获取答卷列表请求
type ListAnswerSheetsRequest struct {
	QuestionnaireCode    string `form:"questionnaire_code"`
	QuestionnaireVersion string `form:"questionnaire_version"`
	WriterID             uint64 `form:"writer_id"`
	TesteeID             uint64 `form:"testee_id"`
	Page                 int    `form:"page" binding:"required,min=1"`
	PageSize             int    `form:"page_size" binding:"required,min=1,max=100"`
}
