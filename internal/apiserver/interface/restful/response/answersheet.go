package response

import "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/viewmodel"

// SaveAnswerSheetResponse 保存答卷响应
type SaveAnswerSheetResponse struct {
	ID uint64 `json:"id"`
}

// GetAnswerSheetResponse 获取答卷响应
type GetAnswerSheetResponse struct {
	ID                   uint64                `json:"id"`
	QuestionnaireCode    string                `json:"questionnaire_code"`
	QuestionnaireVersion string                `json:"questionnaire_version"`
	Title                string                `json:"title"`
	Score                float64               `json:"score"`
	WriterID             uint64                `json:"writer_id"`
	WriterName           string                `json:"writer_name"`
	TesteeID             uint64                `json:"testee_id"`
	TesteeName           string                `json:"testee_name"`
	Answers              []viewmodel.AnswerDTO `json:"answers"`
	CreatedAt            string                `json:"created_at"`
	UpdatedAt            string                `json:"updated_at"`
}

// AnswerSheetItem 答卷列表项
type AnswerSheetItem struct {
	ID                   uint64  `json:"id"`
	QuestionnaireCode    string  `json:"questionnaire_code"`
	QuestionnaireVersion string  `json:"questionnaire_version"`
	Title                string  `json:"title"`
	Score                float64 `json:"score"`
	WriterID             uint64  `json:"writer_id"`
	TesteeID             uint64  `json:"testee_id"`
}

// ListAnswerSheetsResponse 获取答卷列表响应
type ListAnswerSheetsResponse struct {
	Total int64             `json:"total"`
	Items []AnswerSheetItem `json:"items"`
}
