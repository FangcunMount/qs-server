package systemgovernance

import (
	"context"
	"testing"
)

type stubCheckpointReader struct {
	snapshot CheckpointGovernanceSnapshot
	err      error
}

func (s stubCheckpointReader) LoadGovernanceSnapshot(context.Context) (CheckpointGovernanceSnapshot, error) {
	return s.snapshot, s.err
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
