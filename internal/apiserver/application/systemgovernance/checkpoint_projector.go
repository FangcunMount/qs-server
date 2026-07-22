package systemgovernance

import (
	"context"
	"time"
)

// CheckpointView exposes runtime_checkpoint evidence for governance workbench.
type CheckpointView struct {
	GeneratedAt time.Time                    `json:"generated_at"`
	Window      string                       `json:"window"`
	Metrics     MetricsSummary               `json:"metrics"`
	Signals     []Signal                     `json:"signals"`
	Snapshot    CheckpointGovernanceSnapshot `json:"snapshot"`
	Available   bool                         `json:"available"`
	Reason      string                       `json:"reason,omitempty"`
}

type checkpointGovernanceCollector struct {
	reader CheckpointStatusReader
}

func (c checkpointGovernanceCollector) Collect(ctx context.Context, evalCtx evaluationContext) (*CheckpointView, error) {
	view := &CheckpointView{
		GeneratedAt: evalCtx.evalAt,
		Window:      evalCtx.windowLabel,
		Metrics:     evalCtx.metrics,
		Signals:     []Signal{},
	}
	if c.reader == nil {
		view.Reason = "runtime checkpoint reader unavailable"
		return view, nil
	}
	snapshot, err := c.reader.LoadGovernanceSnapshot(ctx)
	if err != nil {
		view.Reason = err.Error()
		return view, nil
	}
	view.Available = true
	view.Snapshot = snapshot
	view.Signals = checkpointSignals(snapshot)
	return view, nil
}

func checkpointSignals(snapshot CheckpointGovernanceSnapshot) []Signal {
	signals := make([]Signal, 0, 1)
	if snapshot.EvaluationRunFailedRetryable > 0 {
		signals = append(signals, Signal{
			ID:       "checkpoint_evaluation_run_retryable_failed",
			Domain:   DomainCheckpoint,
			Severity: SeverityWarning,
			Status:   "retryable_failed_runs_present",
			Title:    "Retryable evaluation runs need attention",
			Evidence: map[string]interface{}{
				"evaluation_run_failed_retryable": snapshot.EvaluationRunFailedRetryable,
			},
		})
	}
	return SortSignals(signals)
}
