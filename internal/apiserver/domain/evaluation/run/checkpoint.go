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

// CheckpointSeam documents the convergence target between evaluation_run and statistics projector checkpoints.
// Table merge is deferred; callers should treat this as the contract for future unification.
type CheckpointSeam interface {
	Begin(scope CheckpointScope, resourceID string, attemptNo int) (bool, error)
	Complete(scope CheckpointScope, resourceID string, attemptNo int, status string, retryable bool, errCode, errMsg string) error
}
