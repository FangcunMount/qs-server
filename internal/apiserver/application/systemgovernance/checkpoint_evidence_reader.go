package systemgovernance

import (
	"context"
	"time"
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

// CheckpointEvidenceReader projects checkpoint snapshots into governance evidence.
type CheckpointEvidenceReader struct {
	reader CheckpointStatusReader
}

func NewCheckpointEvidenceReader(reader CheckpointStatusReader) CheckpointEvidenceReader {
	return CheckpointEvidenceReader{reader: reader}
}

func (r CheckpointEvidenceReader) Snapshot(ctx context.Context, evalAt time.Time) (CheckpointGovernanceSnapshot, bool, error) {
	_ = evalAt
	if r.reader == nil {
		return CheckpointGovernanceSnapshot{}, false, nil
	}
	snapshot, err := r.reader.LoadGovernanceSnapshot(ctx)
	if err != nil {
		return CheckpointGovernanceSnapshot{}, false, err
	}
	return snapshot, true, nil
}
