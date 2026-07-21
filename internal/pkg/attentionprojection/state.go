package attentionprojection

import "time"

// Status is the durable attention projection lifecycle.
type Status string

const (
	StatusPending        Status = "pending"
	StatusSucceeded      Status = "succeeded"
	StatusFailed         Status = "failed"
	StatusManualRequired Status = "manual_required"
)

const (
	// DefaultMaxAttempts is the automatic retry budget before manual_required.
	DefaultMaxAttempts = 10
	// CollectionName stores interpretation report attention projection state.
	CollectionName = "interpretation_attention_projections"
)

// Record is the queryable attention projection state for one report event.
type Record struct {
	EventID      string
	ReportID     string
	AssessmentID string
	TesteeID     uint64
	RiskLevel    string
	MarkKeyFocus bool
	Status       Status
	Attempt      int
	LastError    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PendingInput captures one interpretation.report.generated attention projection.
type PendingInput struct {
	EventID      string
	ReportID     string
	AssessmentID string
	TesteeID     uint64
	RiskLevel    string
	MarkKeyFocus bool
}
