package eventpayload

import "time"

// QuestionnaireChangeAction is a questionnaire lifecycle action.
type QuestionnaireChangeAction string

const (
	QuestionnaireChangeActionPublished   QuestionnaireChangeAction = "published"
	QuestionnaireChangeActionUnpublished QuestionnaireChangeAction = "unpublished"
	QuestionnaireChangeActionArchived    QuestionnaireChangeAction = "archived"
)

// QuestionnaireChangedData is the questionnaire lifecycle changed event body.
type QuestionnaireChangedData struct {
	Code      string                    `json:"code"`
	Version   string                    `json:"version"`
	Title     string                    `json:"title"`
	Action    QuestionnaireChangeAction `json:"action"`
	ChangedAt time.Time                 `json:"changed_at"`
}
