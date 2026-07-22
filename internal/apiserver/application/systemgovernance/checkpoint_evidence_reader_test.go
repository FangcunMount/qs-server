package systemgovernance

import (
	"context"
	"testing"
	"time"
)

type stubCheckpointReader struct {
	snapshot CheckpointGovernanceSnapshot
	err      error
}

func (s stubCheckpointReader) LoadGovernanceSnapshot(context.Context) (CheckpointGovernanceSnapshot, error) {
	return s.snapshot, s.err
}

func TestCheckpointEvidenceReaderReturnsSnapshot(t *testing.T) {
	reader := NewCheckpointEvidenceReader(stubCheckpointReader{
		snapshot: CheckpointGovernanceSnapshot{
			EvaluationRunRunning:         2,
			EvaluationRunFailedRetryable: 1,
		},
	})
	snapshot, ok, err := reader.Snapshot(context.Background(), stubEvalAt())
	if err != nil || !ok {
		t.Fatalf("Snapshot() = (%#v, %v, %v), want available snapshot", snapshot, ok, err)
	}
	if snapshot.EvaluationRunFailedRetryable != 1 || snapshot.EvaluationRunRunning != 2 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestGetOverviewIncludesCheckpointSignals(t *testing.T) {
	view, err := NewFacade(FacadeDeps{
		CheckpointReader: stubCheckpointReader{
			snapshot: CheckpointGovernanceSnapshot{EvaluationRunFailedRetryable: 2},
		},
	}).GetOverview(context.Background(), "5m")
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if view.Checkpoints == nil || !view.Checkpoints.Available {
		t.Fatalf("checkpoints = %#v, want available checkpoint view", view.Checkpoints)
	}
	found := false
	for _, signal := range view.Signals {
		if signal.ID == "checkpoint_evaluation_run_retryable_failed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("signals = %#v, want retryable failed checkpoint signal", view.Signals)
	}
}

func stubEvalAt() time.Time {
	return time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
}
