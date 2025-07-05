package dto

// SaveAnswerSheetRequest 保存答卷请求
type SaveAnswerSheetRequest struct {
	QuestionnaireCode    string   `json:"questionnaire_code" valid:"required"`
	QuestionnaireVersion string   `json:"questionnaire_version" valid:"required"`
	WriterID             uint64   `json:"writer_id" valid:"required"`
	TesteeID             uint64   `json:"testee_id" valid:"required"`
	Answers              []Answer `json:"answers" valid:"required"`
}

//

// SaveAnswerSheetResponse 保存答卷响应
type SaveAnswerSheetResponse struct {
	ID uint64 `json:"id"`
}
