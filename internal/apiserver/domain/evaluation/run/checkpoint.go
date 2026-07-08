package run

import "time"

// CheckpointScope identifies a durable idempotency boundary for projection or execution retries.
type CheckpointScope string

const (
	CheckpointScopeEvaluationRun      CheckpointScope = "evaluation_run"
	CheckpointScopeAnalyticsProjector CheckpointScope = "analytics_projector"
)

// CheckpointRecord is the shared semantic view of write-side run rows and read-side projector checkpoints.
type CheckpointRecord struct {
	Scope        CheckpointScope
	ResourceID   string
	AttemptNo    int
	Status       string
	Retryable    bool
	StartedAt    time.Time
	FinishedAt   *time.Time
	ErrorCode    string
	ErrorMessage string
}

// CheckpointSeam is the unified persistence contract for evaluation runs and analytics projector checkpoints.
type CheckpointSeam interface {
	Begin(scope CheckpointScope, resourceID string, attemptNo int) (bool, error)
	Complete(scope CheckpointScope, resourceID string, attemptNo int, status string, retryable bool, errCode, errMsg string) error
}
