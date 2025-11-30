package questionnaire

// PublishedEventDTO 问卷发布事件 DTO
type PublishedEventDTO struct {
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	QuestionnaireID uint64 `json:"questionnaire_id"`
	Code            string `json:"code"`
	Version         string `json:"version"`
	Title           string `json:"title"`
	PublishedAt     string `json:"published_at"`
}

// UnpublishedEventDTO 问卷下架事件 DTO
type UnpublishedEventDTO struct {
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	QuestionnaireID uint64 `json:"questionnaire_id"`
	Code            string `json:"code"`
	Version         string `json:"version"`
	UnpublishedAt   string `json:"unpublished_at"`
}

// ArchivedEventDTO 问卷归档事件 DTO
type ArchivedEventDTO struct {
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	QuestionnaireID uint64 `json:"questionnaire_id"`
	Code            string `json:"code"`
	Version         string `json:"version"`
	ArchivedAt      string `json:"archived_at"`
}
