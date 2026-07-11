package eventoutcome

import "time"

// ReportGeneratedPayload is the outcome-enriched interpretation report event body.
type ReportGeneratedPayload struct {
	OrgID        int64         `json:"org_id"`
	ReportID     string        `json:"report_id"`
	AssessmentID string        `json:"assessment_id"`
	OutcomeID    string        `json:"outcome_id"`
	TesteeID     uint64        `json:"testee_id"`
	Attempt      uint          `json:"attempt"`
	Model        ModelIdentity `json:"model"`
	PrimaryScore *ScoreValue   `json:"primary_score,omitempty"`
	Level        *ResultLevel  `json:"level,omitempty"`
	GeneratedAt  time.Time     `json:"generated_at"`
}

// IsHighRisk reports whether the outcome should trigger high-risk workflows.
func (d ReportGeneratedPayload) IsHighRisk() bool {
	return LevelIsHighRisk(d.Level)
}

// ReportFailedPayload records one failed interpretation report attempt.
type ReportFailedPayload struct {
	OrgID        int64     `json:"org_id"`
	ReportID     string    `json:"report_id"`
	AssessmentID string    `json:"assessment_id"`
	OutcomeID    string    `json:"outcome_id"`
	TesteeID     uint64    `json:"testee_id"`
	Attempt      uint      `json:"attempt"`
	Reason       string    `json:"reason"`
	FailedAt     time.Time `json:"failed_at"`
}
