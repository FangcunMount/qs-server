package answersheet

// SubmitAnswerSheetRequest 提交答卷请求
type SubmitAnswerSheetRequest struct {
	QuestionnaireCode    string   `json:"questionnaire_code" binding:"required"`
	QuestionnaireVersion string   `json:"questionnaire_version" binding:"required"`
	Title                string   `json:"title"`
	TesteeID             uint64   `json:"testee_id" binding:"required"`
	Answers              []Answer `json:"answers" binding:"required"`
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
	ID      uint64 `json:"id"`
	Message string `json:"message"`
}

// GetAnswerSheetRequest 获取答卷请求
type GetAnswerSheetRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}

// AnswerSheetResponse 答卷响应
type AnswerSheetResponse struct {
	ID                   uint64   `json:"id"`
	QuestionnaireCode    string   `json:"questionnaire_code"`
	QuestionnaireVersion string   `json:"questionnaire_version"`
	Title                string   `json:"title"`
	Score                float64  `json:"score"`
	WriterID             uint64   `json:"writer_id"`
	WriterName           string   `json:"writer_name"`
	TesteeID             uint64   `json:"testee_id"`
	TesteeName           string   `json:"testee_name"`
	Answers              []Answer `json:"answers"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}
