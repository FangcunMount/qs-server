package response

import "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/viewmodel"

// SaveAnswerSheetResponse 保存答卷响应
type SaveAnswerSheetResponse struct {
	ID uint64 `json:"id"`
}

// GetAnswerSheetResponse 获取答卷响应
type GetAnswerSheetResponse struct {
	ID       uint64                `json:"id"`
	Title    string                `json:"title"`
	WriterID uint64                `json:"writer_id"`
	TesteeID uint64                `json:"testee_id"`
	Score    uint16                `json:"score"`
	Answers  []viewmodel.AnswerDTO `json:"answers"`
}
