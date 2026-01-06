package request

// AdminSubmitAnswerSheetRequest 管理员提交答卷请求
type AdminSubmitAnswerSheetRequest struct {
	QuestionnaireCode    string              `json:"questionnaire_code" binding:"required"`
	QuestionnaireVersion string              `json:"questionnaire_version"`
	Title                string              `json:"title"`
	TesteeID             uint64              `json:"testee_id" binding:"required"`
	WriterID             uint64              `json:"writer_id"`
	FillerID             uint64              `json:"filler_id"`
	Answers              []AdminAnswerSubmit `json:"answers" binding:"required"`
}

// AdminAnswerSubmit 管理员提交答案
type AdminAnswerSubmit struct {
	QuestionCode string      `json:"question_code" binding:"required"`
	QuestionType string      `json:"question_type" binding:"required"`
	Value        interface{} `json:"value" binding:"required"`
}

// UpdateScoreRequest 更新分数请求
type UpdateScoreRequest struct {
	AnswerSheetID uint64             `json:"answersheet_id" valid:"required"`
	AnswerScores  map[string]float64 `json:"answer_scores" valid:"required"`
}
