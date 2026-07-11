package eventpayload

import "time"

// EvaluationRequestedData is the event body for an assessment ready to evaluate.
type EvaluationRequestedData struct {
	OrgID             int64     `json:"org_id"`
	AssessmentID      int64     `json:"assessment_id"`
	TesteeID          uint64    `json:"testee_id"`
	QuestionnaireCode string    `json:"questionnaire_code"`
	QuestionnaireVer  string    `json:"questionnaire_version"`
	AnswerSheetID     string    `json:"answersheet_id"`
	ModelKind         string    `json:"model_kind,omitempty"`
	ModelSubKind      string    `json:"model_sub_kind,omitempty"`
	ModelAlgorithm    string    `json:"model_algorithm,omitempty"`
	ModelCode         string    `json:"model_code,omitempty"`
	ModelVersion      string    `json:"model_version,omitempty"`
	ScaleCode         string    `json:"scale_code,omitempty"`
	ScaleVersion      string    `json:"scale_version,omitempty"`
	RequestedAt       time.Time `json:"requested_at"`
}

// NeedsEvaluation reports whether the assessment should be evaluated.
func (d EvaluationRequestedData) NeedsEvaluation() bool {
	return d.ModelCode != "" || d.ScaleCode != ""
}

// EvaluationFailedData is the evaluation failed event body.
type EvaluationFailedData struct {
	OrgID        int64     `json:"org_id"`
	AssessmentID int64     `json:"assessment_id"`
	TesteeID     uint64    `json:"testee_id"`
	Reason       string    `json:"reason"`
	FailedAt     time.Time `json:"failed_at"`
}

// EvaluationOutcomeCommittedData is emitted after Evaluation facts commit.
type EvaluationOutcomeCommittedData struct {
	OrgID           int64     `json:"org_id"`
	AssessmentID    int64     `json:"assessment_id"`
	TesteeID        uint64    `json:"testee_id"`
	OutcomeID       string    `json:"outcome_id"`
	EvaluationRunID string    `json:"evaluation_run_id"`
	CommittedAt     time.Time `json:"committed_at"`
}
