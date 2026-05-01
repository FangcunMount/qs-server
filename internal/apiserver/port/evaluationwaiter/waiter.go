package evaluationwaiter

import "context"

// StatusSummary is the long-polling status payload for an assessment.
type StatusSummary struct {
	Status     string   `json:"status"`
	TotalScore *float64 `json:"total_score,omitempty"`
	RiskLevel  *string  `json:"risk_level,omitempty"`
	UpdatedAt  int64    `json:"updated_at"`
}

// Notifier notifies local waiters after an assessment reaches a terminal stage.
type Notifier interface {
	Notify(ctx context.Context, assessmentID uint64, summary StatusSummary)
	GetWaiterCount(assessmentID uint64) int
}

type Registry interface {
	Notifier
	Add(assessmentID uint64, ch chan StatusSummary)
	Remove(assessmentID uint64, ch chan StatusSummary)
}
