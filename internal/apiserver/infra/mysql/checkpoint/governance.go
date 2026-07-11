package checkpoint

import (
	"context"
	"fmt"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

// GovernanceSnapshot summarizes active runtime_checkpoint rows for operations evidence.
type GovernanceSnapshot struct {
	EvaluationRunRunning         int64 `json:"evaluation_run_running"`
	EvaluationRunFailedRetryable int64 `json:"evaluation_run_failed_retryable"`
	AnalyticsProjectorProcessing int64 `json:"analytics_projector_processing"`
}

func (r *Repository) LoadGovernanceSnapshot(ctx context.Context) (GovernanceSnapshot, error) {
	if r == nil || r.db == nil {
		return GovernanceSnapshot{}, fmt.Errorf("runtime checkpoint repository is not configured")
	}
	var snapshot GovernanceSnapshot
	var err error
	if snapshot.EvaluationRunRunning, err = r.countScopeStatus(ctx, scopeEvaluationRun, evalrun.StatusRunning.String()); err != nil {
		return GovernanceSnapshot{}, err
	}
	if snapshot.EvaluationRunFailedRetryable, err = r.countScopeStatusRetryable(ctx, scopeEvaluationRun, evalrun.StatusFailed.String(), true); err != nil {
		return GovernanceSnapshot{}, err
	}
	if snapshot.AnalyticsProjectorProcessing, err = r.countScopeStatus(
		ctx,
		scopeAnalyticsProjector,
		evalrun.StatusRunning.String(),
	); err != nil {
		return GovernanceSnapshot{}, err
	}
	return snapshot, nil
}

func (r *Repository) countScopeStatus(ctx context.Context, scope, status string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&RuntimeCheckpointPO{}).
		Where("scope = ? AND status = ? AND deleted_at IS NULL", scope, status).
		Count(&count).Error
	return count, err
}

func (r *Repository) countScopeStatusRetryable(ctx context.Context, scope, status string, retryable bool) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&RuntimeCheckpointPO{}).
		Where("scope = ? AND status = ? AND retryable = ? AND deleted_at IS NULL", scope, status, retryable).
		Count(&count).Error
	return count, err
}
