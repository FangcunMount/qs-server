package evaluation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/stretchr/testify/require"
)

func TestPromptEvaluationRecheckPreservesIndependentLifecycle(t *testing.T) {
	createdAt := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
	release := validRelease()
	value, err := NewPromptEvaluationRecheck(
		meta.ID(8101), meta.ID(7101), release.GenerationCaseIDs[0], 1, release,
		12, "user:34", "verify current candidate against one failed source", createdAt,
	)
	require.NoError(t, err)
	require.Equal(t, RecheckStatusQueued, value.Status())

	dispatchedAt := createdAt.Add(time.Second)
	require.NoError(t, value.BeginDispatch("event:8101", dispatchedAt, 5*time.Minute))
	require.Equal(t, RecheckStatusDispatching, value.Status())

	record := generationAttempt(release.GenerationCaseIDs[0], 1, dispatchedAt)
	require.NoError(t, value.Complete("event:8101", record))
	require.Equal(t, RecheckStatusCompleted, value.Status())
	require.NotNil(t, value.Result())
	require.Nil(t, value.Execution())

	restored, err := RestorePromptEvaluationRecheck(PromptEvaluationRecheckPersistedInput{
		ID: value.ID(), SourceRunID: value.SourceRunID(), SourceCaseID: value.SourceCaseID(), SourceAttempt: value.SourceAttempt(),
		Release: value.Release(), Status: value.Status(), Version: value.Version(), Result: value.Result(),
		RequestedOrg: value.RequestedOrgID(), RequestedBy: value.RequestedBy(), Reason: value.Reason(),
		CreatedAt: value.CreatedAt(), FinishedAt: value.FinishedAt(),
	})
	require.NoError(t, err)
	require.Equal(t, value.Status(), restored.Status())
}

func TestPromptEvaluationRecheckDerivesResultUnknownFromEvidence(t *testing.T) {
	createdAt := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	release := validRelease()
	value, err := NewPromptEvaluationRecheck(meta.ID(8102), meta.ID(7102), release.GenerationCaseIDs[0], 1, release, 12, "user:34", "diagnose", createdAt)
	require.NoError(t, err)
	require.NoError(t, value.BeginDispatch("event:8102", createdAt.Add(time.Second), time.Minute))
	record := generationAttempt(release.GenerationCaseIDs[0], 1, createdAt.Add(2*time.Second))
	record.ProviderReceipt = nil
	record.RawOutput = nil
	record.NormalizedOutput = nil
	record.OutputFingerprint = ""
	record.Semantic = nil
	record.Failure = &AttemptFailure{Stage: "provider_execution", Code: "provider_result_unknown", SafeMessage: "Provider result is unknown", ResultUnknown: true}
	record.Assertions = []AssertionReceipt{{Type: "provider_execution", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "runner-v1", Status: AssertionFailed}}
	require.NoError(t, value.Complete("event:8102", record))
	require.Equal(t, RecheckStatusResultUnknown, value.Status())
}
