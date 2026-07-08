package run_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

func TestCheckpointScopesAreDistinctButCompatible(t *testing.T) {
	if run.CheckpointScopeEvaluationRun == run.CheckpointScopeAnalyticsProjector {
		t.Fatal("evaluation_run and analytics_projector scopes must remain distinct until merge")
	}
	record := run.CheckpointRecord{
		Scope:      run.CheckpointScopeEvaluationRun,
		ResourceID: "7001:1",
		AttemptNo:  1,
		Status:     "failed",
		Retryable:  true,
	}
	if record.Scope != run.CheckpointScopeEvaluationRun {
		t.Fatalf("scope = %s", record.Scope)
	}
}
