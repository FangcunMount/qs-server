package eventoutcome

import "time"

// AssessmentInterpretedPayload is the outcome-enriched assessment interpreted event body.
type AssessmentInterpretedPayload struct {
	OrgID         int64         `json:"org_id"`
	AssessmentID  int64         `json:"assessment_id"`
	TesteeID      uint64        `json:"testee_id"`
	Model         ModelIdentity `json:"model"`
	PrimaryScore  *ScoreValue   `json:"primary_score,omitempty"`
	Level         *ResultLevel  `json:"level,omitempty"`
	InterpretedAt time.Time     `json:"interpreted_at"`
}

// IsHighRisk reports whether the outcome should trigger high-risk workflows.
func (d AssessmentInterpretedPayload) IsHighRisk() bool {
	return LevelIsHighRisk(d.Level)
}

// ReportGeneratedPayload is the outcome-enriched report generated event body.
type ReportGeneratedPayload struct {
	ReportID     string        `json:"report_id"`
	AssessmentID string        `json:"assessment_id"`
	TesteeID     uint64        `json:"testee_id"`
	Model        ModelIdentity `json:"model"`
	PrimaryScore *ScoreValue   `json:"primary_score,omitempty"`
	Level        *ResultLevel  `json:"level,omitempty"`
	GeneratedAt  time.Time     `json:"generated_at"`
}

// IsHighRisk reports whether the outcome should trigger high-risk workflows.
func (d ReportGeneratedPayload) IsHighRisk() bool {
	return LevelIsHighRisk(d.Level)
}
