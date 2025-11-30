package assessment

// SavedEventDTO 答卷保存事件 DTO
type SavedEventDTO struct {
	EventID           string `json:"event_id"`
	EventType         string `json:"event_type"`
	AggregateType     string `json:"aggregate_type"`
	AggregateID       string `json:"aggregate_id"`
	OccurredAt        string `json:"occurred_at"`
	AnswerSheetID     uint64 `json:"answer_sheet_id"`
	QuestionnaireCode string `json:"questionnaire_code"`
	QuestionnaireVer  string `json:"questionnaire_ver"`
	TesteeID          uint64 `json:"testee_id"`
	SavedAt           string `json:"saved_at"`
}
