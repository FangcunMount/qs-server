package evaluation_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainoutput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestOnlineRunnerExecutesThirtyFiveAttemptsAndStopsBeforeHumanReview(t *testing.T) {
	provider := &onlineProviderStub{}
	semantic := &onlineSemanticStub{}
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunner(t, promptResolverStub{}, provider, semantic, repository)

	result, err := runner.RunV1(context.Background(), evaluation.OnlineRunCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Preflight == nil || result.Preflight.Status != "passed" || result.Run == nil {
		t.Fatalf("online result = %#v", result)
	}
	if result.Run.Status() != domainevaluation.StatusAwaitingReview || result.Run.IsPublishEvidence() {
		t.Fatalf("run status/publish evidence = %s/%v", result.Run.Status(), result.Run.IsPublishEvidence())
	}
	if provider.calls != domainevaluation.RequiredGenerationAttempts || semantic.calls != domainevaluation.RequiredGenerationAttempts {
		t.Fatalf("provider/semantic calls = %d/%d; deterministic failures = %v", provider.calls, semantic.calls, deterministicFailures(result.Run.Attempts()))
	}
	attempts := result.Run.Attempts()
	if len(attempts) != domainevaluation.RequiredGenerationAttempts+1 {
		t.Fatalf("attempt count = %d", len(attempts))
	}
	invocations := make(map[string]struct{}, provider.calls)
	for _, invocationID := range provider.invocationIDs {
		if _, duplicate := invocations[invocationID]; duplicate {
			t.Fatalf("duplicate invocation id %q", invocationID)
		}
		invocations[invocationID] = struct{}{}
	}

	preflightFound := false
	caseSevenOrdinals := map[int]bool{}
	for _, attempt := range attempts {
		if attempt.Stage == domainevaluation.AttemptStagePreflight {
			preflightFound = attempt.ProviderCallCount == 0 && attempt.RejectionReason == "insufficient_eligible_dimensions"
			continue
		}
		if attempt.Failure != nil || attempt.Semantic == nil || len(attempt.RawOutput) == 0 || len(attempt.NormalizedOutput) == 0 {
			t.Fatalf("generation attempt evidence is incomplete: %#v", attempt)
		}
		if attempt.CaseID == "PROMPT-EVAL-007" && attempt.Attempt == 1 {
			for _, receipt := range attempt.Assertions {
				if receipt.Type == "forbid_dimension_group" && receipt.Scope == domainevaluation.AssertionScopeCase && receipt.Evaluator == "deterministic" {
					caseSevenOrdinals[receipt.Ordinal] = true
				}
			}
		}
	}
	if !preflightFound || !caseSevenOrdinals[1] || !caseSevenOrdinals[2] {
		t.Fatalf("preflight/repeated assertion evidence = %v/%v", preflightFound, caseSevenOrdinals)
	}
}

func deterministicFailures(attempts []domainevaluation.AttemptRecord) map[string][]string {
	result := map[string][]string{}
	for _, attempt := range attempts {
		if attempt.Stage != domainevaluation.AttemptStageGeneration || attempt.Semantic != nil {
			continue
		}
		for _, receipt := range attempt.Assertions {
			if receipt.Status == domainevaluation.AssertionFailed || receipt.Status == domainevaluation.AssertionBlocked {
				result[attempt.CaseID] = append(result[attempt.CaseID], receipt.Type+":"+receipt.Detail)
			}
		}
	}
	return result
}

func TestOnlineRunnerPersistsProviderFailureAndCompletesInventory(t *testing.T) {
	provider := &onlineProviderStub{failAt: 3}
	semantic := &onlineSemanticStub{}
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunner(t, promptResolverStub{}, provider, semantic, repository)

	result, err := runner.RunV1(context.Background(), evaluation.OnlineRunCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.Status() != domainevaluation.StatusAwaitingReview || provider.calls != 35 || semantic.calls != 34 {
		t.Fatalf("run/calls = %s/%d/%d", result.Run.Status(), provider.calls, semantic.calls)
	}
	failures := 0
	for _, attempt := range result.Run.Attempts() {
		if attempt.Failure != nil {
			failures++
			if attempt.Failure.Stage != "provider_execution" || attempt.ProviderCallCount != 1 {
				t.Fatalf("failure evidence = %#v", attempt)
			}
		}
	}
	if failures != 1 {
		t.Fatalf("failure count = %d", failures)
	}
}

func TestOnlineRunnerPersistsSemanticFailureOutcomeWithoutLosingCandidateEvidence(t *testing.T) {
	provider := &onlineProviderStub{}
	semantic := &onlineSemanticStub{
		failAt: 1,
		failure: &domainevaluation.AttemptFailure{
			Stage: string(domainevaluation.FailureStageSemanticEvaluation),
			Code:  domainevaluation.SemanticOutputSchemaInvalid, SafeMessage: "semantic output violated the frozen schema", Retryable: true,
		},
	}
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunner(t, promptResolverStub{}, provider, semantic, repository)

	result, err := runner.RunV1(context.Background(), evaluation.OnlineRunCommand{})
	if err != nil {
		t.Fatal(err)
	}
	var failed *domainevaluation.AttemptRecord
	for _, attempt := range result.Run.Attempts() {
		if attempt.Failure != nil && attempt.Failure.Code == domainevaluation.SemanticOutputSchemaInvalid {
			copy := attempt
			failed = &copy
			break
		}
	}
	if failed == nil || failed.Semantic != nil || failed.SemanticExecution == nil ||
		failed.SemanticExecution.Status() != domainevaluation.ExecutionStatusFailed ||
		failed.SemanticExecution.ProviderReceipt == nil || len(failed.SemanticExecution.RawOutput) == 0 ||
		len(failed.SemanticExecution.NormalizedOutput) == 0 || len(failed.NormalizedOutput) == 0 {
		t.Fatalf("semantic failure evidence = %#v", failed)
	}
}

func TestOnlineRunnerClassifiesInvalidSemanticDecisionsWithoutDroppingExecutionEvidence(t *testing.T) {
	provider := &onlineProviderStub{}
	semantic := &onlineSemanticStub{invalidDecisionsAt: 1}
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunner(t, promptResolverStub{}, provider, semantic, repository)

	result, err := runner.RunV1(context.Background(), evaluation.OnlineRunCommand{})
	if err != nil {
		t.Fatal(err)
	}
	for _, attempt := range result.Run.Attempts() {
		if attempt.Failure == nil || attempt.Failure.Code != domainevaluation.SemanticDecisionContractInvalid {
			continue
		}
		if attempt.SemanticExecution == nil || attempt.SemanticExecution.Failure == nil ||
			attempt.SemanticExecution.ProviderReceipt == nil || len(attempt.SemanticExecution.RawOutput) == 0 {
			t.Fatalf("semantic decision failure evidence = %#v", attempt)
		}
		return
	}
	t.Fatal("semantic decision failure was not recorded")
}

func TestOnlineRunnerPersistsMismatchedSemanticReceiptAsFailureEvidence(t *testing.T) {
	provider := &onlineProviderStub{}
	mismatched := aiexplanation.ProviderReceipt{
		InvocationID: "different-semantic-invocation", RequestID: "judge-mismatch",
		Provider: "different-judge", Model: "different-model", Latency: time.Millisecond,
	}
	semantic := &onlineSemanticStub{
		failAt: 1, receiptOverride: &mismatched,
		failure: &domainevaluation.AttemptFailure{
			Stage: string(domainevaluation.FailureStageSemanticEvaluation),
			Code:  domainevaluation.SemanticReceiptInvalid, SafeMessage: "semantic receipt did not match the frozen execution",
		},
	}
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunner(t, promptResolverStub{}, provider, semantic, repository)

	result, err := runner.RunV1(context.Background(), evaluation.OnlineRunCommand{})
	if err != nil {
		t.Fatal(err)
	}
	for _, attempt := range result.Run.Attempts() {
		if attempt.Failure == nil || attempt.Failure.Code != domainevaluation.SemanticReceiptInvalid {
			continue
		}
		if attempt.SemanticExecution == nil || attempt.SemanticExecution.ProviderReceipt == nil ||
			attempt.SemanticExecution.ProviderReceipt.RequestID != mismatched.RequestID {
			t.Fatalf("mismatched semantic receipt evidence = %#v", attempt.SemanticExecution)
		}
		return
	}
	t.Fatal("mismatched semantic receipt was not persisted")
}

func TestOnlineRunnerRechecksOneAttemptWithoutMutatingSourceEvidence(t *testing.T) {
	provider := &onlineProviderStub{}
	semantic := &onlineSemanticStub{}
	repository := &onlineEvidenceRepository{}
	rechecks := &onlineRecheckRepository{}
	fixed := time.Date(2026, 8, 29, 11, 0, 0, 0, time.UTC)
	runner := newOnlineRunnerWithRepositories(t, promptResolverStub{}, provider, semantic, repository, rechecks, func() time.Time { return fixed })

	prepared, err := runner.PrepareRequestedV1(context.Background(), evaluation.OnlineStartCommand{
		RunID: meta.ID(9201), OrgID: 12, RequestedBy: "user:34", Reason: "source run",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(context.Background(), prepared.Run); err != nil {
		t.Fatal(err)
	}
	step, err := runner.RunStepV1(context.Background(), evaluation.OnlineStepCommand{
		RunID: meta.ID(9201), CaseID: "PROMPT-EVAL-001", Attempt: 1, Owner: "source-event",
		RequestedOrgID: 12, RequestedBy: "user:34",
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceAttemptCount := len(step.Run.Attempts())

	recheck, err := runner.PrepareRequestedRecheckV1(context.Background(), evaluation.PrepareRecheckCommand{
		RecheckID: meta.ID(9301), SourceRunID: meta.ID(9201), CaseID: "PROMPT-EVAL-001", Attempt: 1,
		OrgID: 12, RequestedBy: "user:34", Reason: "verify current candidate",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rechecks.CreateRecheck(context.Background(), recheck); err != nil {
		t.Fatal(err)
	}
	result, err := runner.RunRecheckV1(context.Background(), evaluation.RunRecheckCommand{
		RecheckID: meta.ID(9301), SourceRunID: meta.ID(9201), CaseID: "PROMPT-EVAL-001", Attempt: 1,
		Owner: "recheck-event", RequestedOrg: 12, RequestedBy: "user:34",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != evaluation.OnlineRecheckCompleted || result.Recheck.Status() != domainevaluation.RecheckStatusCompleted ||
		result.Recheck.Result() == nil || result.Recheck.Result().Semantic == nil {
		t.Fatalf("recheck result = %#v", result)
	}
	if len(repository.value.Attempts()) != sourceAttemptCount {
		t.Fatalf("source attempt count changed from %d to %d", sourceAttemptCount, len(repository.value.Attempts()))
	}
	if provider.calls != 2 || semantic.calls != 2 {
		t.Fatalf("source plus recheck Provider calls = %d/%d, want 2/2", provider.calls, semantic.calls)
	}
}

func TestOnlineRunnerClassifiesSchemaInvalidProviderOutputAsTechnicalFailure(t *testing.T) {
	provider := &onlineProviderStub{mutate: func(caseID string, content *domainoutput.Content) {
		if caseID == "PROMPT-EVAL-001" {
			content.SchemaVersion = ""
		}
	}}
	semantic := &onlineSemanticStub{}
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunner(t, promptResolverStub{}, provider, semantic, repository)

	result, err := runner.RunV1(context.Background(), evaluation.OnlineRunCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.Status() != domainevaluation.StatusAwaitingReview || semantic.calls != 30 {
		t.Fatalf("run/semantic calls = %s/%d", result.Run.Status(), semantic.calls)
	}

	failures := 0
	for _, attempt := range result.Run.Attempts() {
		if attempt.CaseID != "PROMPT-EVAL-001" || attempt.Stage != domainevaluation.AttemptStageGeneration {
			continue
		}
		if attempt.Failure == nil || attempt.Failure.Stage != "output_validation" ||
			attempt.Failure.Code != "provider_output_content_contract_invalid" || len(attempt.RawOutput) == 0 || len(attempt.NormalizedOutput) != 0 {
			t.Fatalf("schema-invalid attempt evidence = %#v", attempt)
		}
		failures++
	}
	if failures != 5 || result.Run.FailedAttemptCount() != 5 || !result.Run.CanCancel() {
		t.Fatalf("schema-invalid failure/cancel evidence = %d/%d/%v", failures, result.Run.FailedAttemptCount(), result.Run.CanCancel())
	}
}

func TestOnlineRunnerValidatesNormalizedEnvelopeAndPreservesRawEvidence(t *testing.T) {
	provider := &onlineProviderStub{wrapValidationFence: true}
	semantic := &onlineSemanticStub{}
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunner(t, promptResolverStub{}, provider, semantic, repository)

	result, err := runner.RunV1(context.Background(), evaluation.OnlineRunCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.FailedAttemptCount() != 0 || semantic.calls != domainevaluation.RequiredGenerationAttempts {
		t.Fatalf("failures/semantic calls = %d/%d", result.Run.FailedAttemptCount(), semantic.calls)
	}
	for _, attempt := range result.Run.Attempts() {
		if attempt.Stage != domainevaluation.AttemptStageGeneration {
			continue
		}
		if !strings.HasPrefix(string(attempt.RawOutput), "```json\n") || len(attempt.NormalizedOutput) == 0 || attempt.Failure != nil {
			t.Fatalf("normalized-envelope attempt evidence = %#v", attempt)
		}
	}
}

func TestOnlineRunnerJudgesStructurallyValidCandidateThatFailsDeterministicGate(t *testing.T) {
	provider := &onlineProviderStub{mutate: func(caseID string, content *domainoutput.Content) {
		if caseID == "PROMPT-EVAL-001" {
			content.Suggestions[0].Origin = domainoutput.SuggestionOriginGeneratedLowRisk
			content.Suggestions[0].SourceSuggestionRefs = []string{}
		}
	}}
	semantic := &onlineSemanticStub{}
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunner(t, promptResolverStub{}, provider, semantic, repository)

	started, err := runner.StartV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.RunStepV1(context.Background(), evaluation.OnlineStepCommand{
		RunID: started.Run.ID(), CaseID: "PROMPT-EVAL-001", Attempt: 1, Owner: "event-step-hard-gate",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != evaluation.OnlineStepProgressed || provider.calls != 1 || semantic.calls != 1 {
		attempts := result.Run.Attempts()
		t.Fatalf("step/calls = %s/%d/%d; attempt = %#v", result.Status, provider.calls, semantic.calls, attempts[len(attempts)-1])
	}
	attempts := result.Run.Attempts()
	attempt := attempts[len(attempts)-1]
	if attempt.Failure != nil || attempt.Semantic == nil || len(attempt.NormalizedOutput) == 0 {
		t.Fatalf("hard-gate attempt evidence = %#v", attempt)
	}
	foundDeterministicFailure := false
	for _, receipt := range attempt.Assertions {
		if receipt.Type == "suggestion_origin_present" && receipt.Status == domainevaluation.AssertionFailed {
			foundDeterministicFailure = true
		}
	}
	if !foundDeterministicFailure {
		t.Fatalf("hard-gate failure receipts = %#v", attempt.Assertions)
	}
}

func TestOnlineRunnerPreflightFailureMakesNoProviderCallOrEvidenceRun(t *testing.T) {
	provider := &onlineProviderStub{}
	semantic := &onlineSemanticStub{}
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunner(t, failingPromptResolver{}, provider, semantic, repository)

	result, err := runner.RunV1(context.Background(), evaluation.OnlineRunCommand{})
	if err == nil || result == nil || result.Preflight == nil || result.Preflight.Status != "failed" {
		t.Fatalf("preflight result/error = %#v / %v", result, err)
	}
	if provider.calls != 0 || repository.value != nil {
		t.Fatalf("provider calls/evidence = %d/%#v", provider.calls, repository.value)
	}
}

func TestOnlineRunnerStepIsIdempotentForCompletedEventTarget(t *testing.T) {
	fixed := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	provider := &onlineProviderStub{}
	semantic := &onlineSemanticStub{}
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunnerWithClock(t, promptResolverStub{}, provider, semantic, repository, func() time.Time { return fixed })

	started, err := runner.StartV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	command := evaluation.OnlineStepCommand{
		RunID: started.Run.ID(), CaseID: "PROMPT-EVAL-001", Attempt: 1, Owner: "event-step-1",
	}
	first, err := runner.RunStepV1(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runner.RunStepV1(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != evaluation.OnlineStepProgressed || second.Status != evaluation.OnlineStepAlreadyCompleted || provider.calls != 1 || semantic.calls != 1 {
		t.Fatalf("step results/calls = %s/%s/%d/%d", first.Status, second.Status, provider.calls, semantic.calls)
	}
}

func TestOnlineRunnerExpiredDispatchRecordsUnknownWithoutProviderReplay(t *testing.T) {
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	provider := &onlineProviderStub{}
	semantic := &onlineSemanticStub{}
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunnerWithClock(t, promptResolverStub{}, provider, semantic, repository, func() time.Time { return now })

	started, err := runner.StartV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	leaseUntil := now.Add(time.Minute)
	if err := repository.value.BeginAttemptExecution(domainevaluation.AttemptExecution{
		CaseID: "PROMPT-EVAL-001", Attempt: 1, Owner: "crashed-event", InvocationID: "ai-prompt-eval:9001:PROMPT-EVAL-001:1",
		Phase: domainevaluation.AttemptExecutionPrepared, ClaimedAt: now, LeaseExpiresAt: leaseUntil,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.value.MarkAttemptDispatching("crashed-event", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	now = leaseUntil

	result, err := runner.RunStepV1(context.Background(), evaluation.OnlineStepCommand{
		RunID: started.Run.ID(), CaseID: "PROMPT-EVAL-001", Attempt: 1, Owner: "redelivered-event",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != evaluation.OnlineStepProgressed || provider.calls != 0 || semantic.calls != 0 {
		t.Fatalf("result/calls = %s/%d/%d", result.Status, provider.calls, semantic.calls)
	}
	attempts := result.Run.Attempts()
	last := attempts[len(attempts)-1]
	if last.Failure == nil || last.Failure.Code != "provider_result_unknown" || !last.Failure.ResultUnknown || last.ProviderCallCount != 1 {
		t.Fatalf("unknown dispatch evidence = %#v", last)
	}
}

func newOnlineRunner(
	t *testing.T,
	prompts appport.PromptPackageResolver,
	provider *onlineProviderStub,
	semantic *onlineSemanticStub,
	repository *onlineEvidenceRepository,
) *evaluation.OnlineRunner {
	t.Helper()
	fixed := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	return newOnlineRunnerWithClock(t, prompts, provider, semantic, repository, func() time.Time { return fixed })
}

func newOnlineRunnerWithClock(
	t *testing.T,
	prompts appport.PromptPackageResolver,
	provider *onlineProviderStub,
	semantic *onlineSemanticStub,
	repository *onlineEvidenceRepository,
	now func() time.Time,
) *evaluation.OnlineRunner {
	return newOnlineRunnerWithRepositories(t, prompts, provider, semantic, repository, &onlineRecheckRepository{}, now)
}

func newOnlineRunnerWithRepositories(
	t *testing.T,
	prompts appport.PromptPackageResolver,
	provider *onlineProviderStub,
	semantic *onlineSemanticStub,
	repository *onlineEvidenceRepository,
	rechecks *onlineRecheckRepository,
	now func() time.Time,
) *evaluation.OnlineRunner {
	t.Helper()
	evidence, err := evaluation.NewEvidenceService(repository, func() meta.ID { return meta.ID(9001) }, now)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := evaluation.NewOnlineRunner(evaluation.OnlineRunnerDependencies{
		Prompts: prompts, Schemas: schemaResolverStub{}, Routes: onlineRouteResolver{},
		Provider: provider, Safety: onlineSafetyStub{}, Semantic: semantic, Evidence: evidence,
		AttemptLease: time.Minute, Rechecks: rechecks, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

type onlineRouteResolver struct{}

func (onlineRouteResolver) ResolveProviderRoute(_ context.Context, route string) (appport.ProviderRoute, error) {
	return appport.ProviderRoute{
		ExecutionSpec: aiexplanation.ProviderExecutionSpec{
			Route: route, RouteRevision: "v1", ResolvedProvider: "provider-a", ResolvedModel: "model-a",
			Fingerprint: aiexplanation.NewFingerprint([]byte("online-route-v1")),
		},
		Capabilities: appport.ProviderCapabilities{StructuredOutput: true}, Timeout: time.Second, MaxOutputTokens: 3000,
	}, nil
}

type onlineProviderStub struct {
	calls               int
	failAt              int
	mutate              func(caseID string, content *domainoutput.Content)
	wrapValidationFence bool
	invocationIDs       []string
}

func (p *onlineProviderStub) Generate(_ context.Context, request appport.ProviderRequest) (*appport.ProviderResponse, error) {
	p.calls++
	p.invocationIDs = append(p.invocationIDs, request.InvocationID)
	if p.failAt == p.calls {
		return nil, errors.New("synthetic provider failure")
	}
	caseID := onlineCaseID(request.InvocationID)
	content := onlineCandidate(caseID)
	if p.mutate != nil {
		p.mutate(caseID, &content)
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	response := &appport.ProviderResponse{
		RawOutput: raw,
		Receipt: aiexplanation.ProviderReceipt{
			InvocationID: request.InvocationID, RequestID: "request-" + request.InvocationID,
			Provider: request.Route.ExecutionSpec.ResolvedProvider, Model: request.Route.ExecutionSpec.ResolvedModel,
			InputTokens: 100, OutputTokens: 200, Latency: 10 * time.Millisecond,
		},
	}
	if p.wrapValidationFence {
		response.RawOutput = []byte("```json\n" + string(raw) + "\n```")
		response.ValidationOutput = append([]byte(nil), raw...)
	}
	return response, nil
}

func onlineCaseID(invocationID string) string {
	for index := 1; index <= 7; index++ {
		caseID := "PROMPT-EVAL-00" + string(rune('0'+index))
		if strings.Contains(invocationID, caseID) {
			return caseID
		}
	}
	return ""
}

func onlineCandidate(caseID string) domainoutput.Content {
	refs := map[string][]string{
		"PROMPT-EVAL-001": {"dimension:emotional_awareness", "dimension:self_regulation"},
		"PROMPT-EVAL-002": {"dimension:task_initiation", "dimension:persistence"},
		"PROMPT-EVAL-003": {"dimension:stress_load", "dimension:sleep_recovery"},
		"PROMPT-EVAL-004": {"dimension:flexible_planning", "dimension:detail_checking"},
		"PROMPT-EVAL-005": {"dimension:sleep_recovery", "dimension:daily_energy"},
		"PROMPT-EVAL-006": {"dimension:reflection", "dimension:planning"},
		"PROMPT-EVAL-007": {"dimension:impulse_control", "dimension:attention_shift"},
	}[caseID]
	kind := map[string]domainoutput.InsightKind{
		"PROMPT-EVAL-001": domainoutput.InsightKindReinforcingPattern,
		"PROMPT-EVAL-002": domainoutput.InsightKindContrastingPattern,
		"PROMPT-EVAL-003": domainoutput.InsightKindCombinedAttention,
		"PROMPT-EVAL-004": domainoutput.InsightKindContextDependent,
		"PROMPT-EVAL-005": domainoutput.InsightKindReinforcingPattern,
		"PROMPT-EVAL-006": domainoutput.InsightKindContextDependent,
		"PROMPT-EVAL-007": domainoutput.InsightKindContrastingPattern,
	}[caseID]
	evidenceRefs := []domainoutput.EvidenceRef{
		{Kind: domainoutput.EvidenceKindDimension, Ref: refs[0]},
		{Kind: domainoutput.EvidenceKindDimension, Ref: refs[1]},
	}
	origin := domainoutput.SuggestionOriginGeneratedLowRisk
	sourceRefs := []string{}
	if caseID == "PROMPT-EVAL-001" {
		origin = domainoutput.SuggestionOriginStandardDerived
		sourceRefs = []string{"suggestion:awareness_note"}
	}
	return domainoutput.Content{
		SchemaVersion: aiexplanation.OutputSchemaVersionV1,
		Summary:       "本次两个维度可结合观察，并以保守方式理解其关系。",
		IntegratedInsights: []domainoutput.IntegratedInsight{{
			Kind: kind, Title: "跨维度组合", Content: "本次结果显示，两个维度在部分情境中可能形成可观察的组合关系。",
			WhyItMatters: "结合观察有助于理解本次结果。", EvidenceRefs: evidenceRefs,
		}},
		Suggestions: []domainoutput.Suggestion{{
			Origin: origin, Category: "daily_practice", Title: "小步观察", Goal: "观察两个维度的共同变化",
			Actions: []string{"选择一个日常情境做简短记录"}, Rationale: "该步骤与本次两个维度相关。",
			EvidenceRefs: evidenceRefs, SourceSuggestionRefs: sourceRefs,
		}},
		Limitations: []string{"本解读仅基于本次测评，不构成诊断或确定性判断。"},
	}
}

type onlineSafetyStub struct{}

func (onlineSafetyStub) Evaluate(_ context.Context, _ appport.SafetyRequest) (appport.SafetyResult, error) {
	return appport.SafetyResult{Allowed: true, ValidatorVersion: "test-safety/v1"}, nil
}

type onlineSemanticStub struct {
	calls              int
	failAt             int
	failure            *domainevaluation.AttemptFailure
	invalidDecisionsAt int
	receiptOverride    *aiexplanation.ProviderReceipt
}

func (*onlineSemanticStub) Identity() domainevaluation.SemanticEvaluatorSpec {
	return domainevaluation.SemanticEvaluatorSpec{
		Version: "synthetic-semantic/v1",
		Prompt: aiexplanation.PromptRef{
			TemplateID: "ai-explanation-semantic-evaluator", Version: "v1",
			Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-prompt")), GitBlobSHA: "semantic-prompt-blob",
		},
		OutputSchema: domainevaluation.SchemaRef{Version: "ai-explanation-semantic-evaluation-output/v1", Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-schema"))},
		Provider: aiexplanation.ProviderExecutionSpec{
			Route: "semantic_judge_v1", RouteRevision: "v1", ResolvedProvider: "judge-provider", ResolvedModel: "judge-model",
			Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-route")),
		},
		Decoding: domainevaluation.DecodingParameters{MaxOutputTokens: 2000},
	}
}

func (s *onlineSemanticStub) Evaluate(_ context.Context, request evaluation.SemanticEvaluationRequest) (evaluation.SemanticEvaluationOutcome, error) {
	s.calls++
	decisions := make([]evaluation.SemanticDecision, 0, len(request.Assertions))
	for _, assertion := range request.Assertions {
		decisions = append(decisions, evaluation.SemanticDecision{
			Type: assertion.Type, Scope: assertion.Scope, Ordinal: assertion.Ordinal,
			Status: domainevaluation.AssertionPassed, Detail: "synthetic evaluator passed the frozen obligation",
		})
	}
	if s.invalidDecisionsAt == s.calls {
		decisions = nil
	}
	receipt := aiexplanation.ProviderReceipt{
		InvocationID: request.InvocationID, RequestID: "judge-" + request.InvocationID,
		Provider: "judge-provider", Model: "judge-model", InputTokens: 50, OutputTokens: 50, Latency: 5 * time.Millisecond,
	}
	if s.receiptOverride != nil && (s.failAt == 0 || s.failAt == s.calls) {
		receipt = *s.receiptOverride
	}
	result := evaluation.SemanticEvaluationResult{
		EvaluatorVersion: "synthetic-semantic/v1",
		Scores: domainevaluation.SemanticScores{
			Faithfulness: 5, CrossDimensionQuality: 5, SuggestionActionability: 5, AudienceClarity: 5, Concision: 5,
		},
		Rationale: "synthetic rubric evaluation", Decisions: decisions,
	}
	now := time.Now().UTC()
	raw := []byte(`{"schema_version":"synthetic-semantic/v1"}`)
	outcome := evaluation.SemanticEvaluationOutcome{
		InvocationID: request.InvocationID, EvaluatorVersion: "synthetic-semantic/v1",
		StartedAt: now, FinishedAt: now, ProviderCallCount: 1, ProviderReceipt: &receipt,
		RawOutput: raw, NormalizedOutput: append([]byte(nil), raw...), Result: &result,
	}
	if s.failAt == s.calls && s.failure != nil {
		failure := *s.failure
		outcome.Result = nil
		outcome.Failure = &failure
	}
	return outcome, nil
}

type onlineEvidenceRepository struct {
	value      *domainevaluation.PromptEvaluationRun
	list       []evaluation.ReviewRunCatalogRecord
	nextCursor string
	listOrgID  int64
	listStatus *domainevaluation.Status
	listCursor string
	listLimit  int
}

type onlineRecheckRepository struct {
	values map[meta.ID]*domainevaluation.PromptEvaluationRecheck
}

func (r *onlineRecheckRepository) CreateRecheck(_ context.Context, value *domainevaluation.PromptEvaluationRecheck) error {
	if r.values == nil {
		r.values = map[meta.ID]*domainevaluation.PromptEvaluationRecheck{}
	}
	if _, exists := r.values[value.ID()]; exists {
		return domainevaluation.ErrAlreadyExists
	}
	r.values[value.ID()] = value
	return nil
}

func (r *onlineRecheckRepository) SaveRecheck(_ context.Context, value *domainevaluation.PromptEvaluationRecheck, _ int64) error {
	if r.values == nil {
		r.values = map[meta.ID]*domainevaluation.PromptEvaluationRecheck{}
	}
	r.values[value.ID()] = value
	return nil
}

func (r *onlineRecheckRepository) FindRecheckByID(_ context.Context, id meta.ID) (*domainevaluation.PromptEvaluationRecheck, error) {
	value := r.values[id]
	if value == nil {
		return nil, domainevaluation.ErrNotFound
	}
	return value, nil
}

func (r *onlineRecheckRepository) ListRechecksBySource(_ context.Context, runID meta.ID, caseID string, attempt, _ int) ([]*domainevaluation.PromptEvaluationRecheck, error) {
	values := make([]*domainevaluation.PromptEvaluationRecheck, 0)
	for _, value := range r.values {
		if value.SourceRunID() == runID && value.SourceCaseID() == caseID && value.SourceAttempt() == attempt {
			values = append(values, value)
		}
	}
	return values, nil
}

func (r *onlineEvidenceRepository) Create(_ context.Context, value *domainevaluation.PromptEvaluationRun) error {
	if r.value != nil {
		return domainevaluation.ErrAlreadyExists
	}
	r.value = value
	return nil
}

func (r *onlineEvidenceRepository) Save(_ context.Context, value *domainevaluation.PromptEvaluationRun, _ int64) error {
	r.value = value
	return nil
}

func (r *onlineEvidenceRepository) FindByID(_ context.Context, id meta.ID) (*domainevaluation.PromptEvaluationRun, error) {
	if r.value == nil || r.value.ID() != id {
		return nil, domainevaluation.ErrNotFound
	}
	return r.value, nil
}

func (r *onlineEvidenceRepository) ListForReview(_ context.Context, orgID int64, status *domainevaluation.Status, cursor string, limit int) ([]evaluation.ReviewRunCatalogRecord, string, error) {
	r.listOrgID, r.listStatus, r.listCursor, r.listLimit = orgID, status, cursor, limit
	return append([]evaluation.ReviewRunCatalogRecord(nil), r.list...), r.nextCursor, nil
}

func catalogRecordFromRun(runRecord *domainevaluation.PromptEvaluationRun) evaluation.ReviewRunCatalogRecord {
	record := evaluation.ReviewRunCatalogRecord{
		RunID: runRecord.ID(), Version: runRecord.Version(), Status: runRecord.Status(), Release: runRecord.Release(),
		RequestedOrgID: runRecord.RequestedOrgID(), RequestedBy: runRecord.RequestedBy(),
		RequestReason: runRecord.RequestReason(), CreatedAt: runRecord.CreatedAt(), Gate: runRecord.Gate(),
	}
	for _, attempt := range runRecord.Attempts() {
		record.Attempts = append(record.Attempts, evaluation.ReviewRunCatalogAttempt{
			CaseID: attempt.CaseID, Attempt: attempt.Attempt, Stage: attempt.Stage, Failed: attempt.Failure != nil,
		})
	}
	for _, review := range runRecord.Reviews() {
		record.Reviews = append(record.Reviews, evaluation.ReviewRunCatalogReview{
			CaseID: review.CaseID, Attempt: review.Attempt, Role: review.Role, Decision: review.Decision,
		})
	}
	if execution := runRecord.Execution(); execution != nil {
		phase := execution.Phase
		record.ExecutionPhase = &phase
	}
	return record
}

type failingPromptResolver struct{}

func (failingPromptResolver) ResolvePromptPackage(context.Context, string, string) (appport.PromptPackage, error) {
	return appport.PromptPackage{}, errors.New("synthetic Prompt catalog failure")
}
