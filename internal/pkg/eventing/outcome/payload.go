package eventoutcome

import "time"

// ReportGeneratedPayload is the outcome-enriched interpretation report event body.
type ReportGeneratedPayload struct {
	OrgID                int64         `json:"org_id"`
	GenerationID         string        `json:"generation_id"`
	RunID                string        `json:"run_id"`
	ReportID             string        `json:"report_id"`
	AssessmentID         string        `json:"assessment_id"`
	OutcomeID            string        `json:"outcome_id"`
	TesteeID             uint64        `json:"testee_id"`
	Attempt              uint          `json:"attempt"`
	ReportType           string        `json:"report_type"`
	TemplateVersion      string        `json:"template_version"`
	BuilderIdentity      string        `json:"builder_identity"`
	ContentSchemaVersion string        `json:"content_schema_version"`
	Model                ModelIdentity `json:"model"`
	PrimaryScore         *ScoreValue   `json:"primary_score,omitempty"`
	Level                *ResultLevel  `json:"level,omitempty"`
	GeneratedAt          time.Time     `json:"generated_at"`
}

// IsHighRisk reports whether the outcome should trigger high-risk workflows.
func (d ReportGeneratedPayload) IsHighRisk() bool {
	return LevelIsHighRisk(d.Level)
}

// ReportFailedPayload records one failed interpretation report attempt.
type ReportFailedPayload struct {
	OrgID           int64     `json:"org_id"`
	GenerationID    string    `json:"generation_id"`
	RunID           string    `json:"run_id"`
	AssessmentID    string    `json:"assessment_id"`
	OutcomeID       string    `json:"outcome_id"`
	TesteeID        uint64    `json:"testee_id"`
	Attempt         uint      `json:"attempt"`
	ReportType      string    `json:"report_type"`
	TemplateVersion string    `json:"template_version"`
	FailureKind     string    `json:"failure_kind"`
	FailureCode     string    `json:"failure_code"`
	Retryable       bool      `json:"retryable"`
	SafeReason      string    `json:"safe_reason"`
	FailedAt        time.Time `json:"failed_at"`
}

// InterpretationRetryRequestedPayload wakes a generation only after its
// persisted retry decision becomes due.
type InterpretationRetryRequestedPayload struct {
	OrgID           int64     `json:"org_id"`
	GenerationID    string    `json:"generation_id"`
	RunID           string    `json:"run_id"`
	AssessmentID    string    `json:"assessment_id"`
	OutcomeID       string    `json:"outcome_id"`
	TesteeID        uint64    `json:"testee_id"`
	ExpectedAttempt int       `json:"expected_attempt"`
	AttemptOrigin   string    `json:"attempt_origin"`
	ActionRequestID string    `json:"action_request_id,omitempty"`
	Mode            string    `json:"mode"`
	RequestedAt     time.Time `json:"requested_at"`
}
