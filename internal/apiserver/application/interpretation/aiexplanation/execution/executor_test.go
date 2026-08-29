package execution

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	appinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/input"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainartifact "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domaininput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

func TestExecutePersistsDispatchBoundaryAndGeneratedArtifact(t *testing.T) {
	f := newFixture(t)
	result, err := f.executor.Execute(context.Background(), Command{GenerationID: f.generation.ID(), TraceID: "trace-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusGenerated || result.Artifact == nil || result.Generation.Status() != domaingeneration.StatusGenerated || result.Run.Status() != domainrun.StatusSucceeded {
		t.Fatalf("result = %#v", result)
	}
	wantSequence := []string{"start", "dispatch", "provider", "safety", "success"}
	if stringSlice(*f.sequence) != stringSlice(wantSequence) {
		t.Fatalf("sequence = %v, want %v", *f.sequence, wantSequence)
	}
	request := f.provider.request
	if request.OutputSchema.Version != aiexplanation.OutputSchemaVersionV1 || len(request.OutputSchema.JSON) == 0 || request.InvocationID == "" || len(request.DataJSON) == 0 {
		t.Fatalf("provider request = %#v", request)
	}
	if string(request.DataJSON) == string(f.generation.Input().CanonicalJSON()) {
		t.Fatal("provider received server-side source/profile envelope")
	}
	if result.Artifact.ProviderReceipt().InvocationID != request.InvocationID || result.Artifact.Source().ReportID != f.generation.Key().SourceReportID {
		t.Fatalf("artifact = %#v", result.Artifact)
	}
}

func TestExecuteFailsClosedOnInvalidProviderOutput(t *testing.T) {
	f := newFixture(t)
	f.provider.raw = []byte(`{"schema_version":"ai-explanation-output/v1","summary":"bad"}`)
	result, err := f.executor.Execute(context.Background(), Command{GenerationID: f.generation.ID(), TraceID: "trace-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusFailed || result.Failure == nil || result.Failure.Kind != domainrun.FailureKindOutputValidation || result.Failure.Retryable {
		t.Fatalf("result = %#v", result)
	}
	if len(f.artifacts.byID) != 0 || f.safety.calls != 0 {
		t.Fatalf("artifacts/safety calls = %d/%d", len(f.artifacts.byID), f.safety.calls)
	}
}

func TestExecuteUsesEnvelopeNormalizedProviderOutput(t *testing.T) {
	f := newFixture(t)
	f.provider.validationOutput = append([]byte(nil), f.provider.raw...)
	f.provider.raw = []byte("```json\n" + string(f.provider.raw) + "\n```")

	result, err := f.executor.Execute(context.Background(), Command{GenerationID: f.generation.ID(), TraceID: "trace-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusGenerated || result.Artifact == nil || f.safety.calls != 1 {
		t.Fatalf("result/safety calls = %#v/%d", result, f.safety.calls)
	}
}

func TestExecuteNeverCallsProviderWhenDispatchStateCannotPersist(t *testing.T) {
	f := newFixture(t)
	f.committer.dispatchErr = errors.New("storage unavailable")
	result, err := f.executor.Execute(context.Background(), Command{GenerationID: f.generation.ID(), TraceID: "trace-1"})
	if result != nil || !errors.Is(err, f.committer.dispatchErr) {
		t.Fatalf("result/error = %#v/%v", result, err)
	}
	if f.provider.calls != 0 || stringSlice(*f.sequence) != stringSlice([]string{"start", "dispatch"}) {
		t.Fatalf("provider calls/sequence = %d/%v", f.provider.calls, *f.sequence)
	}
}

func TestExecuteDoesNotReplayAfterProviderResponseWhenTerminalCommitFails(t *testing.T) {
	f := newFixture(t)
	executor := f.executor.(*executor)
	dispatchedAt := executor.now()
	f.committer.successErr = errors.New("terminal transaction unavailable")

	result, err := f.executor.Execute(context.Background(), Command{
		GenerationID: f.generation.ID(), TraceID: "evt-ai-requested-1",
	})
	if result != nil || !errors.Is(err, f.committer.successErr) {
		t.Fatalf("terminal commit result/error = %#v/%v", result, err)
	}
	if f.provider.calls != 1 {
		t.Fatalf("provider calls after terminal commit failure = %d, want 1", f.provider.calls)
	}

	// CommitSuccess is transactional in production. Rebuild the durable state
	// that remains after its rollback: Generation is still generating and the
	// non-idempotent invocation is still dispatching without a durable receipt.
	persistedGeneration, err := domaingeneration.New(domaingeneration.NewInput{
		ID: f.generation.ID(), Key: f.generation.Key(), Association: f.generation.Association(),
		RequestedBy: f.generation.RequestedBy(), Input: f.generation.Input(), Prompt: f.generation.Prompt(),
		ExecutionSpec: f.generation.ExecutionSpec(), CreatedAt: f.generation.CreatedAt(),
	})
	if err != nil {
		t.Fatal(err)
	}
	persistedRun, err := domainrun.NewPending(meta.FromUint64(800), persistedGeneration.ID(), 1, retrygovernance.AttemptOriginInitial)
	if err != nil {
		t.Fatal(err)
	}
	leaseExpiresAt := dispatchedAt.Add(time.Minute)
	if err := persistedRun.StartWithLease(dispatchedAt, "evt-ai-requested-1", leaseExpiresAt, "generation-700/attempt-1"); err != nil {
		t.Fatal(err)
	}
	if err := persistedRun.BeginProviderDispatch(dispatchedAt); err != nil {
		t.Fatal(err)
	}
	if err := persistedGeneration.Begin(persistedRun.ID(), dispatchedAt); err != nil {
		t.Fatal(err)
	}
	f.generations.byID[persistedGeneration.ID()] = persistedGeneration
	f.runs.byID[persistedRun.ID()] = persistedRun

	recoveredAt := leaseExpiresAt.Add(time.Second)
	wakeup := domainrun.RecoveryWakeup{
		EventID: "lease-recovery-after-terminal-rollback", ExpectedLeaseExpiresAt: leaseExpiresAt,
		InvocationPhase: domainrun.InvocationPhaseDispatching, RequestedAt: recoveredAt,
	}
	if created, scheduleErr := persistedRun.ScheduleRecoveryWakeup(wakeup); scheduleErr != nil || !created {
		t.Fatalf("schedule rollback recovery proof = created:%t error:%v", created, scheduleErr)
	}
	executor.now = func() time.Time { return recoveredAt }
	f.committer.successErr = nil

	result, err = f.executor.Execute(context.Background(), Command{
		GenerationID: persistedGeneration.ID(), TraceID: wakeup.EventID, EventID: wakeup.EventID,
		ExpectedRunID: persistedRun.ID(), ExpectedLeaseExpiresAt: leaseExpiresAt,
		ExpectedInvocationPhase: domainrun.InvocationPhaseDispatching,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusFailed || result.Failure == nil || result.Failure.Code != "provider_result_unknown" || result.Failure.Retryable {
		t.Fatalf("recovery result = %#v", result)
	}
	if result.Run.InvocationPhase() != domainrun.InvocationPhaseResultUnknown || f.provider.calls != 1 {
		t.Fatalf("recovery phase/provider calls = %s/%d, want result_unknown/1", result.Run.InvocationPhase(), f.provider.calls)
	}
}

func TestExecuteMarksUnknownNonIdempotentInvocationNonRetryable(t *testing.T) {
	f := newFixture(t)
	f.provider.err = &appport.ProviderError{
		Kind: domainrun.FailureKindProviderTimeout, Code: "provider_timeout", SafeMessage: "AI 解读暂时不可用", Retryable: true, ResultUnknown: true,
	}
	result, err := f.executor.Execute(context.Background(), Command{GenerationID: f.generation.ID(), TraceID: "trace-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusFailed || result.Failure.Retryable || result.Run.InvocationPhase() != domainrun.InvocationPhaseResultUnknown {
		t.Fatalf("result = %#v, phase = %s", result, result.Run.InvocationPhase())
	}
}

func TestExecuteReclaimsExpiredPreparedRunBeforeProviderCall(t *testing.T) {
	f := newFixture(t)
	executor := f.executor.(*executor)
	startedAt := executor.now().Add(-2 * time.Minute)
	runRecord, err := domainrun.NewPending(meta.FromUint64(800), f.generation.ID(), 1, "initial")
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.StartWithLease(startedAt, "old-trace", startedAt.Add(time.Minute), "generation-700/attempt-1"); err != nil {
		t.Fatal(err)
	}
	if err := f.generation.Begin(runRecord.ID(), startedAt); err != nil {
		t.Fatal(err)
	}
	f.runs.byID[runRecord.ID()] = runRecord

	result, err := f.executor.Execute(context.Background(), Command{GenerationID: f.generation.ID(), TraceID: "recovery-trace"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusGenerated || result.Run.RecoveryCount() != 1 || f.provider.calls != 1 {
		t.Fatalf("result=%#v recovery=%d provider_calls=%d", result, result.Run.RecoveryCount(), f.provider.calls)
	}
	if result.Run.TraceID() != "recovery-trace" {
		t.Fatalf("trace = %q", result.Run.TraceID())
	}
	if got := stringSlice(*f.sequence); got != stringSlice([]string{"dispatch", "provider", "safety", "success"}) {
		t.Fatalf("sequence = %s", got)
	}
}

func TestExecuteRequiresPersistedProofForLeaseRecoveryEvent(t *testing.T) {
	f := newFixture(t)
	executor := f.executor.(*executor)
	now := executor.now()
	startedAt := now.Add(-2 * time.Minute)
	runRecord, err := domainrun.NewPending(meta.FromUint64(800), f.generation.ID(), 1, retrygovernance.AttemptOriginInitial)
	if err != nil {
		t.Fatal(err)
	}
	leaseExpiredAt := startedAt.Add(time.Minute)
	if err := runRecord.StartWithLease(startedAt, "old-trace", leaseExpiredAt, "generation-700/attempt-1"); err != nil {
		t.Fatal(err)
	}
	if err := f.generation.Begin(runRecord.ID(), startedAt); err != nil {
		t.Fatal(err)
	}
	f.runs.byID[runRecord.ID()] = runRecord
	command := Command{
		GenerationID: f.generation.ID(), TraceID: "lease-recovery-1", EventID: "lease-recovery-1",
		ExpectedRunID: runRecord.ID(), ExpectedLeaseExpiresAt: leaseExpiredAt,
		ExpectedInvocationPhase: domainrun.InvocationPhasePrepared,
	}
	if _, err := f.executor.Execute(context.Background(), command); !errors.Is(err, domainrun.ErrRecoveryNotAllowed) {
		t.Fatalf("unpersisted recovery proof error = %v", err)
	}
	if f.provider.calls != 0 {
		t.Fatalf("unpersisted recovery called Provider %d times", f.provider.calls)
	}
	wakeup := domainrun.RecoveryWakeup{
		EventID: command.EventID, ExpectedLeaseExpiresAt: leaseExpiredAt,
		InvocationPhase: domainrun.InvocationPhasePrepared, RequestedAt: now,
	}
	if created, err := runRecord.ScheduleRecoveryWakeup(wakeup); err != nil || !created {
		t.Fatalf("schedule recovery proof = created:%t error:%v", created, err)
	}
	result, err := f.executor.Execute(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusGenerated || result.Run.RecoveryCount() != 1 || f.provider.calls != 1 {
		t.Fatalf("recovery result=%#v calls=%d", result, f.provider.calls)
	}
}

func TestExecuteNeverUsesRecoveryProofToStartPendingGeneration(t *testing.T) {
	f := newFixture(t)
	now := f.executor.(*executor).now()
	_, err := f.executor.Execute(context.Background(), Command{
		GenerationID: f.generation.ID(), TraceID: "lease-recovery-1", EventID: "lease-recovery-1",
		ExpectedRunID: meta.FromUint64(800), ExpectedLeaseExpiresAt: now.Add(-time.Minute),
		ExpectedInvocationPhase: domainrun.InvocationPhasePrepared,
	})
	if !errors.Is(err, domainrun.ErrRecoveryNotAllowed) || f.provider.calls != 0 {
		t.Fatalf("pending recovery = error:%v provider_calls:%d", err, f.provider.calls)
	}
}

func TestExecuteFailsExpiredNonIdempotentDispatchWithoutSecondProviderCall(t *testing.T) {
	f := newFixture(t)
	executor := f.executor.(*executor)
	startedAt := executor.now().Add(-2 * time.Minute)
	runRecord, err := domainrun.NewPending(meta.FromUint64(800), f.generation.ID(), 1, "initial")
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.StartWithLease(startedAt, "old-trace", startedAt.Add(time.Minute), "generation-700/attempt-1"); err != nil {
		t.Fatal(err)
	}
	if err := runRecord.BeginProviderDispatch(startedAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := f.generation.Begin(runRecord.ID(), startedAt); err != nil {
		t.Fatal(err)
	}
	f.runs.byID[runRecord.ID()] = runRecord
	leaseExpiredAt := startedAt.Add(time.Minute)
	wakeup := domainrun.RecoveryWakeup{
		EventID: "lease-recovery-dispatch-1", ExpectedLeaseExpiresAt: leaseExpiredAt,
		InvocationPhase: domainrun.InvocationPhaseDispatching, RequestedAt: executor.now(),
	}
	if created, err := runRecord.ScheduleRecoveryWakeup(wakeup); err != nil || !created {
		t.Fatalf("schedule dispatch recovery = created:%t error:%v", created, err)
	}

	result, err := f.executor.Execute(context.Background(), Command{
		GenerationID: f.generation.ID(), TraceID: wakeup.EventID, EventID: wakeup.EventID,
		ExpectedRunID: runRecord.ID(), ExpectedLeaseExpiresAt: leaseExpiredAt,
		ExpectedInvocationPhase: domainrun.InvocationPhaseDispatching,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusFailed || result.Failure == nil || result.Failure.Code != "provider_result_unknown" || result.Failure.Retryable {
		t.Fatalf("result = %#v", result)
	}
	if result.Run.InvocationPhase() != domainrun.InvocationPhaseResultUnknown || result.Run.RecoveryWakeup() != nil || f.provider.calls != 0 {
		t.Fatalf("phase=%s provider_calls=%d", result.Run.InvocationPhase(), f.provider.calls)
	}
}

func TestExecuteStartsAuthorizedManualRetryAsIndependentAttempt(t *testing.T) {
	f := newFixture(t)
	f.provider.err = &appport.ProviderError{Kind: domainrun.FailureKindProviderTimeout, Code: "provider_timeout", SafeMessage: "暂时不可用", Retryable: true}
	failed, err := f.executor.Execute(context.Background(), Command{GenerationID: f.generation.ID(), TraceID: "initial-event"})
	if err != nil || failed.Status != StatusFailed {
		t.Fatalf("initial failure = %#v, error = %v", failed, err)
	}
	authorizedAt := time.Date(2026, 8, 26, 12, 1, 0, 0, time.UTC)
	authorization := domainrun.RetryAuthorization{
		ExpectedAttempt: 1, NextAttempt: 2, Origin: retrygovernance.AttemptOriginManual,
		RequestID: "retry-request-1", EventID: "retry-event-1", Actor: "user:42", Reason: "manual recovery", AuthorizedAt: authorizedAt,
	}
	if err := failed.Run.AuthorizeManualRetry(authorization); err != nil {
		t.Fatal(err)
	}
	f.provider.err = nil
	result, err := f.executor.Execute(context.Background(), Command{
		GenerationID: f.generation.ID(), TraceID: authorization.EventID, EventID: authorization.EventID,
		ExpectedAttempt: 1, AttemptOrigin: retrygovernance.AttemptOriginManual, ActionRequestID: authorization.RequestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusGenerated || result.Run.Attempt() != 2 || result.Run.Origin() != retrygovernance.AttemptOriginManual || f.provider.calls != 2 {
		t.Fatalf("retry result = %#v, provider calls = %d", result, f.provider.calls)
	}
	if result.Run.InvocationID() != "generation-700/attempt-2" {
		t.Fatalf("retry invocation id = %q", result.Run.InvocationID())
	}
}

func TestExecuteRejectsRetryWakeupThatDoesNotMatchPersistedAuthorization(t *testing.T) {
	f := newFixture(t)
	f.provider.err = &appport.ProviderError{Kind: domainrun.FailureKindProviderTimeout, Code: "provider_timeout", SafeMessage: "暂时不可用", Retryable: true}
	failed, err := f.executor.Execute(context.Background(), Command{GenerationID: f.generation.ID(), TraceID: "initial-event"})
	if err != nil || failed.Status != StatusFailed {
		t.Fatalf("initial failure = %#v, error = %v", failed, err)
	}
	authorization := domainrun.RetryAuthorization{
		ExpectedAttempt: 1, NextAttempt: 2, Origin: retrygovernance.AttemptOriginManual,
		RequestID: "retry-request-1", EventID: "retry-event-1", Actor: "user:42", Reason: "manual recovery", AuthorizedAt: time.Date(2026, 8, 26, 12, 1, 0, 0, time.UTC),
	}
	if err := failed.Run.AuthorizeManualRetry(authorization); err != nil {
		t.Fatal(err)
	}
	result, err := f.executor.Execute(context.Background(), Command{
		GenerationID: f.generation.ID(), TraceID: "forged-event", EventID: "forged-event",
		ExpectedAttempt: 1, AttemptOrigin: retrygovernance.AttemptOriginManual, ActionRequestID: authorization.RequestID,
	})
	if result != nil || !errors.Is(err, domainrun.ErrRetryNotAllowed) {
		t.Fatalf("mismatched retry result/error = %#v/%v", result, err)
	}
	if f.provider.calls != 1 {
		t.Fatalf("mismatched retry called provider; calls = %d", f.provider.calls)
	}
}

type fixture struct {
	executor    Executor
	generation  *domaingeneration.AIExplanationGeneration
	generations *generationRepositoryStub
	runs        *runRepositoryStub
	artifacts   *artifactRepositoryStub
	provider    *providerStub
	safety      *safetyStub
	committer   *committerStub
	sequence    *[]string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	definition := validDefinition()
	profileRecord, err := domainprofile.NewDraft(meta.FromUint64(500), definition, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := profileRecord.Publish(meta.ID(101), "tester", "approved evaluation", now.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	profileRef := aiexplanation.ProfileRef{ID: profileRecord.ProfileID(), Version: profileRecord.Version(), Fingerprint: profileRecord.Fingerprint()}
	document := validInputDocument(profileRef, now)
	rawInput, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := domaininput.NewSnapshot(rawInput)
	if err != nil {
		t.Fatal(err)
	}
	promptRef := aiexplanation.PromptRef{TemplateID: definition.GenerationPolicy.PromptTemplateID, Version: definition.GenerationPolicy.PromptVersion, Fingerprint: aiexplanation.NewFingerprint([]byte("prompt")), GitBlobSHA: "abc123"}
	route := appport.ProviderRoute{
		ExecutionSpec: aiexplanation.ProviderExecutionSpec{Route: definition.GenerationPolicy.ProviderRoute, RouteRevision: "v1", ResolvedProvider: "provider-a", ResolvedModel: "model-a", Fingerprint: aiexplanation.NewFingerprint([]byte("route"))},
		Capabilities:  appport.ProviderCapabilities{StructuredOutput: true}, Timeout: 30 * time.Second, MaxOutputTokens: 4096,
	}
	generationRecord, err := domaingeneration.New(domaingeneration.NewInput{
		ID:          meta.FromUint64(700),
		Key:         domaingeneration.Key{SourceReportID: meta.FromUint64(101), Audience: policy.AudienceParticipant, Profile: profileRef, InputFingerprint: snapshot.Fingerprint(), ExecutionSpecFingerprint: route.ExecutionSpec.Fingerprint},
		Association: aiexplanation.Association{OrgID: 1, AssessmentID: meta.FromUint64(7), TesteeID: 9}, RequestedBy: aiexplanation.ActorRef{Kind: "participant", ID: "user-1"},
		Input: snapshot, Prompt: promptRef, ExecutionSpec: route.ExecutionSpec, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	sequence := []string{}
	generations := &generationRepositoryStub{byID: map[meta.ID]*domaingeneration.AIExplanationGeneration{generationRecord.ID(): generationRecord}}
	runs := &runRepositoryStub{byID: map[meta.ID]*domainrun.AIExplanationRun{}}
	artifacts := &artifactRepositoryStub{byID: map[meta.ID]*domainartifact.AIExplanationArtifact{}}
	provider := &providerStub{sequence: &sequence, raw: marshalOutput(t, validOutput())}
	safety := &safetyStub{sequence: &sequence}
	committer := &committerStub{sequence: &sequence, runs: runs, artifacts: artifacts}
	service, err := NewExecutor(
		generations, runs, artifacts, &profileRepositoryStub{profile: profileRecord},
		&promptResolverStub{pkg: appport.PromptPackage{Ref: promptRef, SystemMessage: "system", TaskTemplate: "locale={{locale}}", DataPreamble: "data", AllowedPlaceholders: []string{"{{locale}}"}}},
		&routeResolverStub{route: route}, &schemaResolverStub{}, provider, safety, committer, time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	concrete := service.(*executor)
	concrete.now = func() time.Time { return now }
	ids := []meta.ID{meta.FromUint64(800), meta.FromUint64(900), meta.FromUint64(1000), meta.FromUint64(1100)}
	concrete.newID = func() meta.ID {
		id := ids[0]
		ids = ids[1:]
		return id
	}
	return &fixture{executor: service, generation: generationRecord, generations: generations, runs: runs, artifacts: artifacts, provider: provider, safety: safety, committer: committer, sequence: &sequence}
}

func validDefinition() domainprofile.Definition {
	return domainprofile.Definition{
		SchemaVersion: aiexplanation.ProfileSchemaVersionV1, ProfileID: "participant-scale", Version: "v1",
		Selector:         domainprofile.Selector{Audience: policy.AudienceParticipant, ModelKind: modelcatalog.KindScale, DecisionKind: modelcatalog.DecisionKindScoreRange},
		Eligibility:      domainprofile.EligibilityPolicy{MinEligibleDimensions: 2, MaxInputDimensions: 12, OnDimensionOverflow: "reject"},
		InputPolicy:      domainprofile.InputPolicy{ContextScope: "current_assessment_only"},
		InsightPolicy:    domainprofile.InsightPolicy{AllowedKinds: []output.InsightKind{output.InsightKindReinforcingPattern}, MinItems: 1, MaxItems: 2, MinDimensionRefsPerItem: 2, MaxDimensionRefsPerItem: 3},
		SuggestionPolicy: domainprofile.SuggestionPolicy{AllowedOrigins: []output.SuggestionOrigin{output.SuggestionOriginStandardDerived}, AllowedCategories: []string{"daily_practice"}, MinItems: 1, MaxItems: 2, MaxActionsPerItem: 2, RequireEvidenceRefs: true, RequireStandardRefsForStandardDerived: true},
		SafetyPolicy:     domainprofile.SafetyPolicy{PolicyVersion: "v1", DisclaimerVersion: "v1", ForbiddenClaims: []string{"diagnosis", "causality", "medication", "treatment_plan", "risk_reclassification", "identity_inference", "deterministic_future_prediction"}},
		GenerationPolicy: domainprofile.GenerationPolicy{PromptTemplateID: "cross-dimension-participant-scale", PromptVersion: "v1", ProviderRoute: "balanced_text_v1", InputSchemaVersion: aiexplanation.InputSchemaVersionV1, OutputSchemaVersion: aiexplanation.OutputSchemaVersionV1, MaxOutputCharacters: 8000},
	}
}

func validInputDocument(profile aiexplanation.ProfileRef, now time.Time) appinput.Document {
	return appinput.Document{
		SchemaVersion: aiexplanation.InputSchemaVersionV1,
		Source:        appinput.Source{ReportID: "101", OutcomeID: "301", ReportType: "standard", TemplateVersion: "v1", ContentSchemaVersion: "v1", BuilderIdentity: "factor-scoring", GeneratedAt: now.Add(-time.Hour).Format(time.RFC3339Nano)},
		Profile:       appinput.ProfileRef{ProfileID: profile.ID, ProfileVersion: profile.Version, ProfileFingerprint: profile.Fingerprint.String()},
		Context:       appinput.Context{Scope: "current_assessment_only", Audience: "participant", Locale: "zh-CN", PersonalizationScope: "assessment_result_only", FocusAreas: []string{}},
		Facts: appinput.Facts{
			Runtime: appinput.Runtime{DecisionKind: "score_range"}, Model: appinput.Model{Kind: "scale", Algorithm: "scale_default", Code: "model-a", Version: "v1", Title: "示例量表"},
			Dimensions:          []appinput.Dimension{{Ref: "dimension:sleep", Code: "sleep"}, {Ref: "dimension:stress", Code: "stress"}},
			StandardSuggestions: []appinput.StandardSuggestion{{Ref: "suggestion:sleep-note", Category: "dimension", Content: "记录睡眠", DimensionRefs: []string{"dimension:sleep"}}},
		},
	}
}

func validOutput() output.Content {
	return output.Content{
		SchemaVersion: aiexplanation.OutputSchemaVersionV1, Summary: "睡眠与压力可以结合观察。",
		IntegratedInsights: []output.IntegratedInsight{{Kind: output.InsightKindReinforcingPattern, Title: "组合关注", Content: "两个维度可能相互影响。", WhyItMatters: "有助于理解本次结果。", EvidenceRefs: []output.EvidenceRef{{Kind: output.EvidenceKindDimension, Ref: "dimension:sleep"}, {Kind: output.EvidenceKindDimension, Ref: "dimension:stress"}}}},
		Suggestions:        []output.Suggestion{{Origin: output.SuggestionOriginStandardDerived, Category: "daily_practice", Title: "记录节律", Goal: "观察变化", Actions: []string{"每天记录一次"}, Rationale: "来自本次标准建议。", EvidenceRefs: []output.EvidenceRef{{Kind: output.EvidenceKindStandardSuggestion, Ref: "suggestion:sleep-note"}}, SourceSuggestionRefs: []string{"suggestion:sleep-note"}}},
		Limitations:        []string{"仅基于本次测评，不构成诊断或确定性判断。"},
	}
}

func marshalOutput(t *testing.T, content output.Content) []byte {
	t.Helper()
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type generationRepositoryStub struct {
	byID map[meta.ID]*domaingeneration.AIExplanationGeneration
}

func (*generationRepositoryStub) Create(context.Context, *domaingeneration.AIExplanationGeneration) error {
	return nil
}
func (s *generationRepositoryStub) FindByID(_ context.Context, id meta.ID) (*domaingeneration.AIExplanationGeneration, error) {
	if value := s.byID[id]; value != nil {
		return value, nil
	}
	return nil, domaingeneration.ErrNotFound
}
func (*generationRepositoryStub) FindByKey(context.Context, domaingeneration.Key) (*domaingeneration.AIExplanationGeneration, error) {
	return nil, domaingeneration.ErrNotFound
}
func (s *generationRepositoryStub) Save(_ context.Context, value *domaingeneration.AIExplanationGeneration, _ uint64) error {
	s.byID[value.ID()] = value
	return nil
}

type runRepositoryStub struct {
	byID map[meta.ID]*domainrun.AIExplanationRun
}

func (s *runRepositoryStub) Create(_ context.Context, value *domainrun.AIExplanationRun) error {
	s.byID[value.ID()] = value
	return nil
}
func (s *runRepositoryStub) FindByID(_ context.Context, id meta.ID) (*domainrun.AIExplanationRun, error) {
	if value := s.byID[id]; value != nil {
		return value, nil
	}
	return nil, domainrun.ErrNotFound
}
func (s *runRepositoryStub) FindLatestByGenerationID(_ context.Context, id meta.ID) (*domainrun.AIExplanationRun, error) {
	for _, value := range s.byID {
		if value.GenerationID() == id {
			return value, nil
		}
	}
	return nil, domainrun.ErrNotFound
}
func (s *runRepositoryStub) Save(_ context.Context, value *domainrun.AIExplanationRun) error {
	s.byID[value.ID()] = value
	return nil
}
func (s *runRepositoryStub) ReclaimExpiredLease(_ context.Context, id meta.ID, at time.Time, traceID string, leaseUntil time.Time, allowIdempotentRedispatch bool) (*domainrun.AIExplanationRun, bool, error) {
	value := s.byID[id]
	if value == nil {
		return nil, false, nil
	}
	if err := value.ReclaimExpiredLease(at, traceID, leaseUntil, allowIdempotentRedispatch); err != nil {
		return nil, false, err
	}
	s.byID[id] = value
	return value, true, nil
}

type artifactRepositoryStub struct {
	byID map[meta.ID]*domainartifact.AIExplanationArtifact
}

func (s *artifactRepositoryStub) Insert(_ context.Context, value *domainartifact.AIExplanationArtifact) error {
	s.byID[value.ID()] = value
	return nil
}
func (s *artifactRepositoryStub) FindByID(_ context.Context, id meta.ID) (*domainartifact.AIExplanationArtifact, error) {
	if value := s.byID[id]; value != nil {
		return value, nil
	}
	return nil, domainartifact.ErrNotFound
}
func (*artifactRepositoryStub) FindByGenerationID(context.Context, meta.ID) (*domainartifact.AIExplanationArtifact, error) {
	return nil, domainartifact.ErrNotFound
}
func (*artifactRepositoryStub) FindBySourceReportAndAudience(context.Context, meta.ID, policy.Audience) (*domainartifact.AIExplanationArtifact, error) {
	return nil, domainartifact.ErrNotFound
}

type profileRepositoryStub struct {
	profile *domainprofile.AIExplanationProfile
}

func (*profileRepositoryStub) Save(context.Context, *domainprofile.AIExplanationProfile) error {
	return nil
}
func (s *profileRepositoryStub) FindByKey(context.Context, string, string) (*domainprofile.AIExplanationProfile, error) {
	return s.profile, nil
}
func (*profileRepositoryStub) ListPublishedByBaseSelector(context.Context, policy.Audience, modelcatalog.Kind, modelcatalog.DecisionKind) ([]*domainprofile.AIExplanationProfile, error) {
	return nil, nil
}

type promptResolverStub struct{ pkg appport.PromptPackage }

func (s *promptResolverStub) ResolvePromptPackage(context.Context, string, string) (appport.PromptPackage, error) {
	return s.pkg, nil
}

type routeResolverStub struct{ route appport.ProviderRoute }

func (s *routeResolverStub) ResolveFrozenProviderRoute(context.Context, aiexplanation.ProviderExecutionSpec) (appport.ProviderRoute, error) {
	return s.route, nil
}

type schemaResolverStub struct{}

func (*schemaResolverStub) ResolveOutputSchema(context.Context, string) (appport.StructuredOutputSchema, error) {
	raw := []byte(`{"type":"object"}`)
	return appport.StructuredOutputSchema{Version: aiexplanation.OutputSchemaVersionV1, Name: "AIExplanationOutput v1", JSON: raw, Fingerprint: aiexplanation.NewFingerprint(raw)}, nil
}

type providerStub struct {
	sequence         *[]string
	raw              []byte
	validationOutput []byte
	err              error
	request          appport.ProviderRequest
	calls            int
}

func (s *providerStub) Generate(_ context.Context, request appport.ProviderRequest) (*appport.ProviderResponse, error) {
	*s.sequence = append(*s.sequence, "provider")
	s.calls++
	s.request = request
	if s.err != nil {
		return nil, s.err
	}
	return &appport.ProviderResponse{RawOutput: s.raw, ValidationOutput: s.validationOutput, Receipt: aiexplanation.ProviderReceipt{InvocationID: request.InvocationID, RequestID: "request-1", Provider: request.Route.ExecutionSpec.ResolvedProvider, Model: request.Route.ExecutionSpec.ResolvedModel, InputTokens: 100, OutputTokens: 200, Latency: time.Second}}, nil
}

type safetyStub struct {
	sequence *[]string
	calls    int
}

func (s *safetyStub) Evaluate(context.Context, appport.SafetyRequest) (appport.SafetyResult, error) {
	*s.sequence = append(*s.sequence, "safety")
	s.calls++
	return appport.SafetyResult{Allowed: true, ValidatorVersion: "safety-test/v1"}, nil
}

type committerStub struct {
	sequence    *[]string
	runs        *runRepositoryStub
	artifacts   *artifactRepositoryStub
	dispatchErr error
	successErr  error
}

func (s *committerStub) CommitStart(ctx context.Context, _ *domaingeneration.AIExplanationGeneration, run *domainrun.AIExplanationRun, _ uint64) error {
	*s.sequence = append(*s.sequence, "start")
	return s.runs.Create(ctx, run)
}
func (s *committerStub) SaveDispatching(ctx context.Context, run *domainrun.AIExplanationRun) error {
	*s.sequence = append(*s.sequence, "dispatch")
	if s.dispatchErr != nil {
		return s.dispatchErr
	}
	return s.runs.Save(ctx, run)
}
func (s *committerStub) CommitSuccess(ctx context.Context, _ *domaingeneration.AIExplanationGeneration, _ *domainrun.AIExplanationRun, artifact *domainartifact.AIExplanationArtifact, _ uint64) error {
	*s.sequence = append(*s.sequence, "success")
	if s.successErr != nil {
		return s.successErr
	}
	return s.artifacts.Insert(ctx, artifact)
}
func (s *committerStub) CommitFailure(context.Context, *domaingeneration.AIExplanationGeneration, *domainrun.AIExplanationRun, uint64) error {
	*s.sequence = append(*s.sequence, "failure")
	return nil
}

func stringSlice(values []string) string { raw, _ := json.Marshal(values); return string(raw) }
