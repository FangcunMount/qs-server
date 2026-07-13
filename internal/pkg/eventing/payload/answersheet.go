package eventpayload

import "time"

// AnswerSheetSubmittedData is the answer sheet submitted event body.
type AnswerSheetSubmittedData struct {
	AnswerSheetID        string    `json:"answersheet_id"`
	QuestionnaireCode    string    `json:"questionnaire_code"`
	QuestionnaireVersion string    `json:"questionnaire_version"`
	TesteeID             uint64    `json:"testee_id"`
	OrgID                uint64    `json:"org_id"`
	FillerID             uint64    `json:"filler_id"`
	FillerType           string    `json:"filler_type"`
	TaskID               string    `json:"task_id,omitempty"`
	SubmittedAt          time.Time `json:"submitted_at"`
}
