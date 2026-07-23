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
	ModelAlgorithm    string    `json:"model_algorithm,omitempty"`
	ModelCode         string    `json:"model_code,omitempty"`
	ModelVersion      string    `json:"model_version,omitempty"`
	ScaleCode         string    `json:"scale_code,omitempty"`
	ScaleVersion      string    `json:"scale_version,omitempty"`
	RequestedAt       time.Time `json:"requested_at"`
	ExpectedAttempt   int       `json:"expected_attempt,omitempty"`
	AttemptOrigin     string    `json:"attempt_origin,omitempty"`
	ActionRequestID   string    `json:"action_request_id,omitempty"`
	Mode              string    `json:"mode,omitempty"`
}

// PayloadGateClass classifies evaluation.requested payloads for Worker dispatch
// (EV-R015). Canonical Assessment.NeedsEvaluation decides whether scoring runs;
// this gate only decides whether the event is valid to forward.
type PayloadGateClass string

const (
	// PayloadGateComplete has model or legacy scale identity on the wire.
	PayloadGateComplete PayloadGateClass = "complete"
	// PayloadGateLegacyIncomplete has assessment_id but no model/scale code.
	// Historical questionnaire-only or damaged payloads land here; Worker must
	// still call Execute so Assessment is authoritative.
	PayloadGateLegacyIncomplete PayloadGateClass = "legacy_incomplete"
	// PayloadGateInvalid lacks a usable assessment_id and must not be ACK'd
	// as a successful no-op.
	PayloadGateInvalid PayloadGateClass = "invalid"
)

// HasModelIdentity reports whether the payload carries model or legacy scale code.
func (d EvaluationRequestedData) HasModelIdentity() bool {
	return d.ModelCode != "" || d.ScaleCode != ""
}

// ClassifyPayloadGate returns the EV-R015 dispatch class for this payload.
func (d EvaluationRequestedData) ClassifyPayloadGate() PayloadGateClass {
	if d.AssessmentID <= 0 {
		return PayloadGateInvalid
	}
	if d.HasModelIdentity() {
		return PayloadGateComplete
	}
	return PayloadGateLegacyIncomplete
}

// NeedsEvaluation is retained for log fields and older call sites. Prefer
// ClassifyPayloadGate / HasModelIdentity; Worker must not use this as an ACK gate.
func (d EvaluationRequestedData) NeedsEvaluation() bool {
	return d.HasModelIdentity()
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
