package systemgovernance

import (
	"context"
)

// CheckpointStatusReader loads runtime_checkpoint evidence for governance views.
type CheckpointStatusReader interface {
	LoadGovernanceSnapshot(ctx context.Context) (CheckpointGovernanceSnapshot, error)
}

// CheckpointGovernanceSnapshot summarizes active checkpoint rows by scope.
type CheckpointGovernanceSnapshot struct {
	EvaluationRunRunning         int64
	EvaluationRunFailedRetryable int64
}
