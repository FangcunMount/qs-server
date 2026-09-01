package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainai "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	evaluationevents "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/events"
)

func TestDurableCommitterV2ReservesFrozenWorstCaseAndStagesExactNextActions(t *testing.T) {
	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	value := newServiceEvidenceV2(t)
	repository := &evidenceV2RepositoryStub{}
	stager := &evaluationEventStagerStub{}
	postCommit := &evaluationPostCommitStub{}
	capacity := &capacityRepositoryStub{}
	committer, err := NewDurableCommitterV2(
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
			stager.insideTransaction = true
			capacity.insideTransaction = true
			defer func() {
				stager.insideTransaction = false
				capacity.insideTransaction = false
			}()
			return fn(ctx)
		}),
		repository, evaluationevents.Factory{}, stager, postCommit, capacity, 280, func() time.Time { return now },
	)
	require.NoError(t, err)
	require.NoError(t, committer.CommitStartV2(context.Background(), value))
	require.True(t, stager.stagedInTransaction)
	require.True(t, capacity.reservedInTransaction)
	require.Len(t, capacity.reservations, 1)
	require.Equal(t, value.ExecutionPolicy.WorstCaseProviderCalls(), capacity.reservations[0].ProviderInvocations)
	require.Equal(t, 140, capacity.reservations[0].ProviderInvocations)
	require.Equal(t, 1, postCommit.calls)
	require.Len(t, stager.events, 1)

	firstAction, err := value.NextAction()
	require.NoError(t, err)
	assertEvaluationStepV2Event(t, stager.events[0], value.RunID.String(), firstAction)

	service, err := NewEvidenceV2Service(repository)
	require.NoError(t, err)
	claimedAt := now.Add(time.Minute)
	_, err = service.ClaimNextExecution(context.Background(), value.RunID, ClaimEvidenceV2ExecutionCommand{
		ExecutionID: "generation:case-1:slot-1:1", Owner: stager.events[0].EventID(),
		InvocationID: "invocation:generation:case-1:slot-1:1", ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(time.Minute),
	})
	require.NoError(t, err)
	dispatchAt := claimedAt.Add(10 * time.Second)
	_, err = service.MarkExecutionDispatching(context.Background(), value.RunID, stager.events[0].EventID(), dispatchAt)
	require.NoError(t, err)
	finishedAt := dispatchAt.Add(time.Second)
	normalized := []byte(`{"summary":"candidate"}`)
	execution := domainevaluation.CandidateGenerationExecution{
		ID: "generation:case-1:slot-1:1", CaseID: "case-1", SlotOrdinal: 1, ExecutionOrdinal: 1,
		InvocationID: "invocation:generation:case-1:slot-1:1", Status: domainevaluation.ExecutionStatusSucceeded,
		StartedAt: dispatchAt, FinishedAt: &finishedAt, ProviderCallCount: 1,
		ProviderReceipt: &domainai.ProviderReceipt{
			InvocationID: "invocation:generation:case-1:slot-1:1", RequestID: "provider-request-1",
			Provider: "provider-a", Model: "model-a", Latency: time.Second,
		},
		RawOutput: normalized, NormalizedOutput: normalized,
		NormalizedOutputFingerprint: domainai.NewFingerprint(normalized),
	}
	updated, err := committer.CommitGenerationV2(context.Background(), value.RunID, CompleteGenerationV2Command{
		Owner: stager.events[0].EventID(), CandidateID: "candidate:case-1:slot-1",
		Assertions: []domainevaluation.AssertionReceipt{{
			Type: "output_schema_valid", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1,
			Hard: true, Evaluator: "deterministic-v1", Status: domainevaluation.AssertionPassed,
		}},
		Execution: execution,
	})
	require.NoError(t, err)
	require.Len(t, stager.events, 2)
	require.Equal(t, 2, postCommit.calls)
	nextAction, err := updated.NextAction()
	require.NoError(t, err)
	require.Equal(t, domainevaluation.EvidenceNextActionSemantic, nextAction.Kind)
	require.Equal(t, "candidate:case-1:slot-1", nextAction.CandidateID)
	assertEvaluationStepV2Event(t, stager.events[1], value.RunID.String(), nextAction)
}

func TestDurableCommitterV2RejectsBudgetBelowFrozenPolicy(t *testing.T) {
	value := newServiceEvidenceV2(t)
	committer, err := NewDurableCommitterV2(
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }),
		&evidenceV2RepositoryStub{}, evaluationevents.Factory{}, &evaluationEventStagerStub{}, &evaluationPostCommitStub{},
		&capacityRepositoryStub{}, 139, time.Now,
	)
	require.NoError(t, err)
	require.Error(t, committer.CommitStartV2(context.Background(), value))
}

func TestDurableCommitterV2StagesReplacementAtomicallyAfterResultUnknownAcknowledgement(t *testing.T) {
	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	value := newServiceEvidenceV2(t)
	repository := &evidenceV2RepositoryStub{}
	stager := &evaluationEventStagerStub{}
	postCommit := &evaluationPostCommitStub{}
	committer, err := NewDurableCommitterV2(
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }),
		repository, evaluationevents.Factory{}, stager, postCommit, &capacityRepositoryStub{}, 280, func() time.Time { return now },
	)
	require.NoError(t, err)
	require.NoError(t, committer.CommitStartV2(context.Background(), value))
	service, err := NewEvidenceV2Service(repository)
	require.NoError(t, err)
	owner := stager.events[0].EventID()
	claimedAt := now.Add(time.Minute)
	_, err = service.ClaimNextExecution(context.Background(), value.RunID, ClaimEvidenceV2ExecutionCommand{
		ExecutionID: "generation:case-1:slot-1:1", Owner: owner,
		InvocationID: "invocation:generation:case-1:slot-1:1", ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(time.Minute),
	})
	require.NoError(t, err)
	dispatchAt := claimedAt.Add(10 * time.Second)
	_, err = service.MarkExecutionDispatching(context.Background(), value.RunID, owner, dispatchAt)
	require.NoError(t, err)
	finishedAt := dispatchAt.Add(time.Second)
	unknown, err := committer.CommitGenerationV2(context.Background(), value.RunID, CompleteGenerationV2Command{
		Owner: owner,
		Execution: domainevaluation.CandidateGenerationExecution{
			ID: "generation:case-1:slot-1:1", CaseID: "case-1", SlotOrdinal: 1, ExecutionOrdinal: 1,
			InvocationID: "invocation:generation:case-1:slot-1:1", Status: domainevaluation.ExecutionStatusResultUnknown,
			StartedAt: dispatchAt, FinishedAt: &finishedAt, ProviderCallCount: 1,
			Failure: &domainevaluation.ClassifiedFailure{
				SchemaVersion: domainevaluation.FailureTaxonomySchemaVersionV1,
				Stage:         domainevaluation.FailureStageGenerationExecution, Kind: domainevaluation.FailureKindResultUnknown,
				Code: "provider_result_unknown", ResultUnknown: true, Disposition: domainevaluation.FailureDispositionManualAcknowledgement,
				SafeMessage: "Provider result cannot be determined", EvidenceRefs: []string{"generation:case-1:slot-1:1"},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, domainevaluation.EvidenceStatusBlocked, unknown.Status)
	require.Len(t, stager.events, 1, "result_unknown must not stage an automatic replay")

	resolvedAt := finishedAt.Add(time.Minute)
	resolved, err := committer.CommitResultUnknownResolutionV2(context.Background(), value.RunID, domainevaluation.ResultUnknownResolution{
		ExecutionID: "generation:case-1:slot-1:1", Decision: domainevaluation.ResultUnknownAuthorizeReplacement,
		Actor: "user:42", Reason: "manual inspection cannot prove a result",
		AcknowledgedDuplicateCallAndCostRisk: true, ResolvedAt: resolvedAt,
	})
	require.NoError(t, err)
	require.Equal(t, domainevaluation.EvidenceStatusCollecting, resolved.Status)
	require.Zero(t, resolved.UnresolvedResultUnknownCount)
	require.Len(t, stager.events, 2)
	require.Equal(t, 2, postCommit.calls)
	action, err := resolved.NextAction()
	require.NoError(t, err)
	require.Equal(t, domainevaluation.EvidenceNextActionGeneration, action.Kind)
	require.Equal(t, 2, action.ExecutionOrdinal)
	assertEvaluationStepV2Event(t, stager.events[1], value.RunID.String(), action)
}

func assertEvaluationStepV2Event(t *testing.T, value interface{ EventID() string }, runID string, action domainevaluation.EvidenceNextAction) {
	t.Helper()
	typed, ok := value.(evaluationevents.PromptEvaluationStepEvent)
	require.True(t, ok)
	require.Equal(t, evaluationevents.PromptEvaluationStepV2EventID(runID, action), typed.EventID())
	require.Equal(t, "v2", typed.Data.EvidenceVersion)
	require.Equal(t, string(action.Kind), typed.Data.ExecutionKind)
	require.Equal(t, action.CaseID, typed.Data.CaseID)
	require.Equal(t, action.SlotOrdinal, typed.Data.SlotOrdinal)
	require.Equal(t, action.CandidateID, typed.Data.CandidateID)
	require.Equal(t, action.ExecutionOrdinal, typed.Data.ExecutionOrdinal)
	require.Zero(t, typed.Data.Attempt)
	require.LessOrEqual(t, len(typed.EventID()), 256)
}
