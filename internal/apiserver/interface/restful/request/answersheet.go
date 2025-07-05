package request

import "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/viewmodel"

// SaveAnswerSheetRequest 保存答卷请求
type SaveAnswerSheetRequest struct {
	QuestionnaireCode    string                `json:"questionnaire_code" valid:"required"`
	QuestionnaireVersion string                `json:"questionnaire_version" valid:"required"`
	Title                string                `json:"title" valid:"required"`
	WriterID             uint64                `json:"writer_id" valid:"required"`
	TesteeID             uint64                `json:"testee_id" valid:"required"`
	Answers              []viewmodel.AnswerDTO `json:"answers" valid:"required"`
}
