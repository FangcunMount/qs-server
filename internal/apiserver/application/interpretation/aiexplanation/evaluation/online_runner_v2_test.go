package evaluation_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	evaluationevents "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/events"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestOnlineRunnerV2SeparatesGenerationSemanticAndACKsRedelivery(t *testing.T) {
	clock := &onlineV2Clock{now: time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC)}
	provider := &onlineProviderStub{}
	semantic := &onlineSemanticStub{}
	runner, _, stager := newOnlineRunnerV2(t, clock, provider, semantic)
	started := startOnlineRunV2(t, runner, meta.ID(9501))
	first := started.Evidence
	if first.Release.Suite.ID != evaluation.SuiteIDV3 || first.Release.Suite.Version != evaluation.SuiteVersionV3 ||
		first.Release.Prompt.Version != "v3" || first.Release.Profile.Version != "v3" {
		t.Fatalf("new Evidence v2 Run did not freeze Prompt/Suite/Profile v3: %#v", first.Release)
	}
	firstAction, err := first.NextAction()
	if err != nil {
		t.Fatal(err)
	}
	if len(stager.events) != 1 {
		t.Fatalf("initial v2 events = %d", len(stager.events))
	}
	firstCommand := onlineV2Command(first, firstAction, stager.events[0].EventID())

	generation, err := runner.RunStepV2(context.Background(), firstCommand)
	if err != nil {
		t.Fatal(err)
	}
	if generation.Status != evaluation.OnlineStepV2Progressed || provider.calls != 1 || semantic.calls != 0 || len(stager.events) != 2 {
		t.Fatalf("generation status/calls/events = %s/%d/%d/%d", generation.Status, provider.calls, semantic.calls, len(stager.events))
	}
	semanticAction, err := generation.Evidence.NextAction()
	if err != nil {
		t.Fatal(err)
	}
	if semanticAction.Kind != domainevaluation.EvidenceNextActionSemantic || semanticAction.CandidateID == "" {
		t.Fatalf("semantic action = %#v", semanticAction)
	}
	semanticResult, err := runner.RunStepV2(context.Background(), onlineV2Command(generation.Evidence, semanticAction, stager.events[1].EventID()))
	if err != nil {
		t.Fatal(err)
	}
	if semanticResult.Status != evaluation.OnlineStepV2Progressed || provider.calls != 1 || semantic.calls != 1 || len(stager.events) != 3 {
		t.Fatalf("semantic status/calls/events = %s/%d/%d/%d", semanticResult.Status, provider.calls, semantic.calls, len(stager.events))
	}
	if !semanticResult.Evidence.Slots[0].Candidate.ReviewReady || semanticResult.Evidence.Slots[0].Candidate.AcceptedSemanticExecutionID == "" {
		t.Fatalf("review-ready Candidate = %#v", semanticResult.Evidence.Slots[0].Candidate)
	}
	next, err := semanticResult.Evidence.NextAction()
	if err != nil || next.Kind != domainevaluation.EvidenceNextActionGeneration || next.SlotOrdinal != 2 {
		t.Fatalf("next Slot action = %#v / %v", next, err)
	}

	redelivered, err := runner.RunStepV2(context.Background(), firstCommand)
	if err != nil {
		t.Fatal(err)
	}
	if redelivered.Status != evaluation.OnlineStepV2AlreadyCompleted || provider.calls != 1 || semantic.calls != 1 {
		t.Fatalf("redelivery status/calls = %s/%d/%d", redelivered.Status, provider.calls, semantic.calls)
	}
}

func TestOnlineRunnerV2RetriesOnlySemanticExecution(t *testing.T) {
	clock := &onlineV2Clock{now: time.Date(2026, 9, 1, 2, 0, 0, 0, time.UTC)}
	provider := &onlineProviderStub{}
	semantic := &onlineSemanticStub{
		failAt: 1,
		failure: &domainevaluation.AttemptFailure{
			Stage: string(domainevaluation.FailureStageSemanticEvaluation), Code: domainevaluation.SemanticOutputSchemaInvalid,
			SafeMessage: "semantic output violated the frozen schema", Retryable: true,
		},
	}
	runner, _, stager := newOnlineRunnerV2(t, clock, provider, semantic)
	started := startOnlineRunV2(t, runner, meta.ID(9502))
	firstAction, _ := started.Evidence.NextAction()
	generation, err := runner.RunStepV2(context.Background(), onlineV2Command(started.Evidence, firstAction, stager.events[0].EventID()))
	if err != nil {
		t.Fatal(err)
	}
	semanticOne, _ := generation.Evidence.NextAction()
	failed, err := runner.RunStepV2(context.Background(), onlineV2Command(generation.Evidence, semanticOne, stager.events[1].EventID()))
	if err != nil {
		t.Fatal(err)
	}
	semanticTwo, err := failed.Evidence.NextAction()
	if err != nil || semanticTwo.Kind != domainevaluation.EvidenceNextActionSemantic || semanticTwo.ExecutionOrdinal != 2 ||
		semanticTwo.CandidateID != semanticOne.CandidateID {
		t.Fatalf("semantic retry action = %#v / %v", semanticTwo, err)
	}
	if provider.calls != 1 || semantic.calls != 1 || len(failed.Evidence.GenerationExecutions) != 1 || len(failed.Evidence.SemanticExecutions) != 1 {
		t.Fatalf("first semantic failure calls/evidence = %d/%d/%d/%d", provider.calls, semantic.calls, len(failed.Evidence.GenerationExecutions), len(failed.Evidence.SemanticExecutions))
	}
	succeeded, err := runner.RunStepV2(context.Background(), onlineV2Command(failed.Evidence, semanticTwo, stager.events[2].EventID()))
	if err != nil {
		t.Fatal(err)
	}
	if succeeded.Status != evaluation.OnlineStepV2Progressed || provider.calls != 1 || semantic.calls != 2 ||
		len(succeeded.Evidence.GenerationExecutions) != 1 || len(succeeded.Evidence.SemanticExecutions) != 2 ||
		!succeeded.Evidence.Slots[0].Candidate.ReviewReady {
		t.Fatalf("semantic-only recovery = status:%s calls:%d/%d evidence:%d/%d candidate:%#v",
			succeeded.Status, provider.calls, semantic.calls, len(succeeded.Evidence.GenerationExecutions),
			len(succeeded.Evidence.SemanticExecutions), succeeded.Evidence.Slots[0].Candidate)
	}
}

func TestOnlineRunnerV2KeepsSemanticEvidenceAfterDeterministicSafetyFailure(t *testing.T) {
	clock := &onlineV2Clock{now: time.Date(2026, 9, 1, 2, 30, 0, 0, time.UTC)}
	provider := &onlineProviderStub{}
	semantic := &onlineSemanticStub{}
	runner, _, stager := newOnlineRunnerV2WithSafety(t, clock, provider, semantic, onlineRejectingSafetyStub{})
	started := startOnlineRunV2(t, runner, meta.ID(9504))
	firstAction, err := started.Evidence.NextAction()
	if err != nil {
		t.Fatal(err)
	}

	generation, err := runner.RunStepV2(context.Background(), onlineV2Command(started.Evidence, firstAction, stager.events[0].EventID()))
	if err != nil {
		t.Fatal(err)
	}
	semanticAction, err := generation.Evidence.NextAction()
	if err != nil || semanticAction.Kind != domainevaluation.EvidenceNextActionSemantic {
		t.Fatalf("semantic action after deterministic safety failure = %#v / %v", semanticAction, err)
	}
	completed, err := runner.RunStepV2(context.Background(), onlineV2Command(generation.Evidence, semanticAction, stager.events[1].EventID()))
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != evaluation.OnlineStepV2Progressed || provider.calls != 1 || semantic.calls != 1 ||
		len(completed.Evidence.SemanticExecutions) != 1 || !completed.Evidence.Slots[0].Candidate.ReviewReady {
		t.Fatalf("semantic evidence after safety failure = status:%s calls:%d/%d executions:%d candidate:%#v",
			completed.Status, provider.calls, semantic.calls, len(completed.Evidence.SemanticExecutions), completed.Evidence.Slots[0].Candidate)
	}
}

func TestOnlineRunnerV2TurnsExpiredDispatchIntoResultUnknownWithoutProviderReplay(t *testing.T) {
	clock := &onlineV2Clock{now: time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)}
	provider := &onlineProviderStub{}
	semantic := &onlineSemanticStub{}
	runner, service, stager := newOnlineRunnerV2(t, clock, provider, semantic)
	started := startOnlineRunV2(t, runner, meta.ID(9503))
	action, _ := started.Evidence.NextAction()
	owner := stager.events[0].EventID()
	claimedAt := clock.now
	_, err := service.ClaimNextExecution(context.Background(), started.Evidence.RunID, evaluation.ClaimEvidenceV2ExecutionCommand{
		ExecutionID: "expired:generation:1", Owner: owner, InvocationID: "expired:invocation:1",
		ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.MarkExecutionDispatching(context.Background(), started.Evidence.RunID, owner, claimedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	clock.now = claimedAt.Add(2 * time.Minute)
	result, err := runner.RunStepV2(context.Background(), onlineV2Command(started.Evidence, action, owner))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != evaluation.OnlineStepV2Blocked || provider.calls != 0 || semantic.calls != 0 ||
		result.Evidence.UnresolvedResultUnknownCount != 1 || len(result.Evidence.GenerationExecutions) != 1 || len(stager.events) != 1 {
		t.Fatalf("result_unknown outcome = status:%s calls:%d/%d unknown:%d executions:%d events:%d",
			result.Status, provider.calls, semantic.calls, result.Evidence.UnresolvedResultUnknownCount,
			len(result.Evidence.GenerationExecutions), len(stager.events))
	}
	failure := result.Evidence.GenerationExecutions[0].Failure
	if failure == nil || failure.Kind != domainevaluation.FailureKindResultUnknown ||
		failure.Disposition != domainevaluation.FailureDispositionManualAcknowledgement {
		t.Fatalf("result_unknown failure = %#v", failure)
	}
}

func newOnlineRunnerV2(
	t *testing.T,
	clock *onlineV2Clock,
	provider *onlineProviderStub,
	semantic *onlineSemanticStub,
) (*evaluation.OnlineRunner, *evaluation.EvidenceV2Service, *onlineV2EventStager) {
	return newOnlineRunnerV2WithSafety(t, clock, provider, semantic, onlineSafetyStub{})
}

func newOnlineRunnerV2WithSafety(
	t *testing.T,
	clock *onlineV2Clock,
	provider *onlineProviderStub,
	semantic *onlineSemanticStub,
	safety appport.SafetyEvaluator,
) (*evaluation.OnlineRunner, *evaluation.EvidenceV2Service, *onlineV2EventStager) {
	runner, evidence, stager, _, _ := newOnlineReleaseHarness(t, clock, provider, semantic, safety)
	return runner, evidence, stager
}

func newOnlineReleaseHarness(
	t *testing.T,
	clock *onlineV2Clock,
	provider *onlineProviderStub,
	semantic *onlineSemanticStub,
	safety appport.SafetyEvaluator,
) (*evaluation.OnlineRunner, *evaluation.EvidenceV2Service, *onlineV2EventStager, *evaluation.DurableCommitterV2, *onlineEvidenceV2Repository) {
	t.Helper()
	v1Evidence, err := evaluation.NewEvidenceService(&onlineEvidenceRepository{}, func() meta.ID { return meta.ID(9001) }, clock.Time)
	if err != nil {
		t.Fatal(err)
	}
	v2Repository := &onlineEvidenceV2Repository{}
	v2Evidence, err := evaluation.NewEvidenceV2Service(v2Repository)
	if err != nil {
		t.Fatal(err)
	}
	stager := &onlineV2EventStager{}
	committer, err := evaluation.NewDurableCommitterV2(
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }),
		v2Repository, evaluationevents.Factory{}, stager, onlineV2PostCommit{}, onlineV2Capacity{}, 280, clock.Time,
	)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := evaluation.NewOnlineRunner(evaluation.OnlineRunnerDependencies{
		Prompts: promptResolverStub{}, Schemas: schemaResolverStub{}, Routes: onlineRouteResolver{},
		Provider: provider, Safety: safety, Semantic: semantic, SemanticTimeout: time.Minute,
		AttemptLease: time.Minute, Evidence: v1Evidence, EvidenceV2: v2Evidence, DurableCommitterV2: committer, Now: clock.Time,
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner, v2Evidence, stager, committer, v2Repository
}

type onlineRejectingSafetyStub struct{}

func (onlineRejectingSafetyStub) Evaluate(_ context.Context, _ appport.SafetyRequest) (appport.SafetyResult, error) {
	return appport.SafetyResult{
		Allowed: false, ValidatorVersion: "test-safety/v1", FailureCode: "limitations_incomplete",
		SafeMessage: "candidate limitations are incomplete",
	}, nil
}

func startOnlineRunV2(t *testing.T, runner *evaluation.OnlineRunner, runID meta.ID) *evaluation.OnlineRunV2Result {
	t.Helper()
	result, err := runner.StartRequestedV2(context.Background(), evaluation.OnlineStartV2Command{
		RunID: runID, OrgID: 12, RequestedBy: "user:42", Reason: "verify lightweight v2",
		ExecutionPolicy: onlineExecutionPolicyV2(), GatePolicy: onlineGatePolicyV2(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func onlineV2Command(
	value *domainevaluation.PromptEvaluationEvidenceV2,
	action domainevaluation.EvidenceNextAction,
	owner string,
) evaluation.OnlineStepV2Command {
	kind := domainevaluation.EvidenceExecutionGeneration
	if action.Kind == domainevaluation.EvidenceNextActionSemantic {
		kind = domainevaluation.EvidenceExecutionSemantic
	}
	return evaluation.OnlineStepV2Command{
		RunID: value.RunID, ExecutionKind: kind, CaseID: action.CaseID, SlotOrdinal: action.SlotOrdinal,
		CandidateID: action.CandidateID, ExecutionOrdinal: action.ExecutionOrdinal, Owner: owner,
		RequestedOrgID: value.Audit.OrganizationID, RequestedBy: value.Audit.RequestedBy,
	}
}

type onlineV2Clock struct{ now time.Time }

func (c *onlineV2Clock) Time() time.Time { return c.now }

type onlineEvidenceV2Repository struct {
	value *domainevaluation.PromptEvaluationEvidenceV2
}

func (r *onlineEvidenceV2Repository) CreateEvidenceV2(_ context.Context, value *domainevaluation.PromptEvaluationEvidenceV2) error {
	if r.value != nil {
		return domainevaluation.ErrAlreadyExists
	}
	r.value = cloneOnlineEvidenceV2(value)
	return nil
}

func (r *onlineEvidenceV2Repository) SaveEvidenceV2(_ context.Context, value *domainevaluation.PromptEvaluationEvidenceV2, expectedVersion int64) error {
	if r.value == nil || r.value.Version() != expectedVersion {
		return domainevaluation.ErrConflict
	}
	r.value = cloneOnlineEvidenceV2(value)
	return nil
}

func (r *onlineEvidenceV2Repository) FindEvidenceV2ByID(_ context.Context, id meta.ID) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	if r.value == nil || r.value.RunID != id {
		return nil, domainevaluation.ErrNotFound
	}
	return cloneOnlineEvidenceV2(r.value), nil
}

func cloneOnlineEvidenceV2(value *domainevaluation.PromptEvaluationEvidenceV2) *domainevaluation.PromptEvaluationEvidenceV2 {
	if value == nil {
		return nil
	}
	restored, err := domainevaluation.RestorePromptEvaluationEvidenceV2(value.Clone(), value.Version(), value.Execution())
	if err != nil {
		panic(err)
	}
	return restored
}

type onlineV2EventStager struct{ events []event.DomainEvent }

func (s *onlineV2EventStager) Stage(_ context.Context, values ...event.DomainEvent) error {
	s.events = append(s.events, values...)
	return nil
}

type onlineV2PostCommit struct{}

func (onlineV2PostCommit) AfterCommit(context.Context, []event.DomainEvent, time.Time) {}

type onlineV2Capacity struct{}

func (onlineV2Capacity) EnsureDailyBucket(context.Context, int64, time.Time, time.Time) error {
	return nil
}

func (onlineV2Capacity) ReserveDailyProviderInvocations(_ context.Context, value domainevaluation.DailyCapacityReservation) error {
	if value.ProviderInvocations != 140 {
		return fmt.Errorf("v2 reservation = %d", value.ProviderInvocations)
	}
	return nil
}

func onlineExecutionPolicyV2() domainevaluation.EvaluationExecutionPolicy {
	return domainevaluation.EvaluationExecutionPolicy{
		SchemaVersion: domainevaluation.EvaluationExecutionPolicySchemaVersionV1,
		PolicyID:      "release-evaluation-bounded-recovery", Version: "v1",
		SlotPolicy: domainevaluation.EvaluationSlotPolicy{
			RequiredGenerationCases: domainevaluation.RequiredGenerationCaseCount, RequiredCandidatesPerCase: domainevaluation.RequiredRepetitionsPerCase,
			RequiredPreflightCases: 1, CandidateSelection: domainevaluation.CandidateSelectionFirstContractConformant,
		},
		Generation: domainevaluation.GenerationExecutionBudget{MaxExecutionsPerSlot: 2, MaxExecutionsPerRun: 70},
		Semantic:   domainevaluation.SemanticExecutionBudget{MaxExecutionsPerCandidate: 2, MaxExecutionsPerRun: 70},
		Recovery: domainevaluation.EvaluationRecoveryPolicy{
			AutoRetryableStageCodes: []domainevaluation.FailureSelector{
				{Stage: domainevaluation.FailureStageGenerationExecution, Code: "provider_rate_limited"},
				{Stage: domainevaluation.FailureStageSemanticEvaluation, Code: domainevaluation.SemanticOutputSchemaInvalid},
			},
			ManualRecoveryStageCodes:                   []domainevaluation.FailureSelector{{Stage: domainevaluation.FailureStageGenerationExecution, Code: "provider_result_unknown"}},
			ResultUnknownRequiresManualAcknowledgement: true,
		},
	}
}

func onlineGatePolicyV2() domainevaluation.ReleaseGatePolicy {
	return domainevaluation.ReleaseGatePolicy{
		SchemaVersion: domainevaluation.ReleaseGatePolicySchemaVersionV1, PolicyID: "release-gates", Version: "v1",
		ReleaseIdentity: domainevaluation.ReleaseIdentityGatePolicy{
			RequiredComponents: []domainevaluation.ReleaseIdentityComponent{
				domainevaluation.ReleaseComponentSuite, domainevaluation.ReleaseComponentPrompt, domainevaluation.ReleaseComponentProfile,
				domainevaluation.ReleaseComponentInputSchema, domainevaluation.ReleaseComponentOutputSchema, domainevaluation.ReleaseComponentGenerationRoute,
				domainevaluation.ReleaseComponentSemanticPrompt, domainevaluation.ReleaseComponentSemanticOutputSchema,
				domainevaluation.ReleaseComponentSemanticRoute, domainevaluation.ReleaseComponentExecutionPolicy,
			},
			RequireFingerprintMatch: true,
		},
		SampleCompleteness: domainevaluation.SampleCompletenessGatePolicy{
			RequiredGenerationCases: 7, RequiredCandidatesPerCase: 5, RequiredCandidateCount: 35,
			RequiredSemanticReceiptsPerCandidate: 1, RejectUnresolvedResultUnknown: true, RejectBudgetOverrun: true,
		},
		ExecutionReliability: domainevaluation.ExecutionReliabilityGatePolicy{
			MinInfrastructureSuccessRate: 0.98, MinGenerationContractConformanceRate: 0.95, MinSemanticExecutionSuccessRate: 0.98,
			InfrastructureDenominator: "dispatched_provider_executions", GenerationContractDenominator: "definite_output_generation_executions",
			SemanticExecutionDenominator: "dispatched_semantic_executions", IncludeResultUnknownInInfrastructureDenominator: true,
		},
		CandidateQuality: domainevaluation.CandidateQualityGatePolicy{
			MinAssertionPassesPerCase: 4, MinAssertionPassesOverall: 32,
			MinimumSemanticScores:              domainevaluation.SemanticScoreThresholds{Faithfulness: 4, CrossDimensionQuality: 3, SuggestionActionability: 3, AudienceClarity: 3, Concision: 3},
			MinimumSemanticAverages:            domainevaluation.SemanticScoreThresholds{Faithfulness: 4.5, CrossDimensionQuality: 4, SuggestionActionability: 4, AudienceClarity: 4, Concision: 4},
			HardAssertionFailureRejectsRelease: true,
		},
		HumanAccountability: domainevaluation.HumanAccountabilityGatePolicy{
			RequiredRoles:               []domainevaluation.ReviewRole{domainevaluation.ReviewRoleAssessmentSemantics, domainevaluation.ReviewRoleSafetyProduct},
			RequiredReviewsPerCandidate: 2, RequiredReviewCount: 70, RequireDistinctReviewersPerCandidate: true,
			RequireReason: true, AnyRejectionRejectsRelease: true,
		},
		ApprovalRule: "all_gates_must_pass",
	}
}

var _ appport.Provider = (*onlineProviderStub)(nil)
