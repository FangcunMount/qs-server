package platform

import (
	"context"

	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
)

type checkpointGovernanceAdapter struct {
	repo *checkpoint.Repository
}

func NewCheckpointGovernanceReader(repo *checkpoint.Repository) systemgov.CheckpointStatusReader {
	if repo == nil {
		return nil
	}
	return checkpointGovernanceAdapter{repo: repo}
}

func (a checkpointGovernanceAdapter) LoadGovernanceSnapshot(ctx context.Context) (systemgov.CheckpointGovernanceSnapshot, error) {
	snapshot, err := a.repo.LoadGovernanceSnapshot(ctx)
	if err != nil {
		return systemgov.CheckpointGovernanceSnapshot{}, err
	}
	return systemgov.CheckpointGovernanceSnapshot{
		EvaluationRunRunning:         snapshot.EvaluationRunRunning,
		EvaluationRunFailedRetryable: snapshot.EvaluationRunFailedRetryable,
		AnalyticsProjectorProcessing: snapshot.AnalyticsProjectorProcessing,
	}, nil
}
