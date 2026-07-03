package eventpayload

import "time"

// AssessmentSubmittedData is the assessment submitted event body.
type AssessmentSubmittedData struct {
	OrgID             int64     `json:"org_id"`
	AssessmentID      int64     `json:"assessment_id"`
	TesteeID          uint64    `json:"testee_id"`
	QuestionnaireCode string    `json:"questionnaire_code"`
	QuestionnaireVer  string    `json:"questionnaire_version"`
	AnswerSheetID     string    `json:"answersheet_id"`
	ModelKind         string    `json:"model_kind,omitempty"`
	ModelCode         string    `json:"model_code,omitempty"`
	ModelVersion      string    `json:"model_version,omitempty"`
	ScaleCode         string    `json:"scale_code,omitempty"`
	ScaleVersion      string    `json:"scale_version,omitempty"`
	SubmittedAt       time.Time `json:"submitted_at"`
}

// NeedsEvaluation reports whether the assessment should be evaluated.
func (d AssessmentSubmittedData) NeedsEvaluation() bool {
	return d.ModelCode != "" || d.ScaleCode != ""
}

// AssessmentFailedData is the assessment failed event body.
type AssessmentFailedData struct {
	OrgID        int64     `json:"org_id"`
	AssessmentID int64     `json:"assessment_id"`
	TesteeID     uint64    `json:"testee_id"`
	Reason       string    `json:"reason"`
	FailedAt     time.Time `json:"failed_at"`
}
