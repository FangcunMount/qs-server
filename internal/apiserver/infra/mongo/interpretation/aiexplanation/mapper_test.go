package aiexplanation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainartifact "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domaininput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"go.mongodb.org/mongo-driver/bson"
)

func TestProfileMapperRoundTripsThroughBSON(t *testing.T) {
	mapper := NewMapper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	draftDefinition := mapperProfile(t, now).Definition()
	draftDefinition.ProfileID = "participant-scale-empty-array-fixture"
	draftDefinition.Eligibility.EligibleDimensionCodes = []string{}
	draftDefinition.Eligibility.ExcludedDimensionCodes = []string{}
	draft, err := domainprofile.NewDraftForRelease(meta.FromUint64(501), draftDefinition, "user:41", "initial release candidate", now)
	if err != nil {
		t.Fatal(err)
	}

	for name, profileRecord := range map[string]*domainprofile.AIExplanationProfile{
		"draft suite fixture": draft,
		"published":           mapperProfile(t, now),
	} {
		t.Run(name, func(t *testing.T) {
			profilePO, mapErr := mapper.ProfileToPO(profileRecord)
			if mapErr != nil {
				t.Fatal(mapErr)
			}
			payload, marshalErr := bson.Marshal(profilePO)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var restoredPO ProfilePO
			if unmarshalErr := bson.Unmarshal(payload, &restoredPO); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
			restoredProfile, restoreErr := mapper.ProfileToDomain(&restoredPO)
			if restoreErr != nil {
				t.Fatalf("restore Profile after BSON round trip: %v; persisted=%#v", restoreErr, restoredPO)
			}
			if restoredProfile.Fingerprint() != profileRecord.Fingerprint() || restoredProfile.Status() != profileRecord.Status() || restoredProfile.CreatedBy() != profileRecord.CreatedBy() {
				t.Fatalf("restored Profile = %#v", restoredProfile)
			}
		})
	}
}

func TestMapperRoundTripsLifecycleAndImmutableArtifact(t *testing.T) {
	mapper := NewMapper()
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	profileRecord := mapperProfile(t, now)
	profilePO, err := mapper.ProfileToPO(profileRecord)
	if err != nil {
		t.Fatal(err)
	}
	restoredProfile, err := mapper.ProfileToDomain(profilePO)
	if err != nil {
		t.Fatal(err)
	}
	if restoredProfile.Fingerprint() != profileRecord.Fingerprint() || restoredProfile.Status() != domainprofile.StatusPublished || restoredProfile.CreatedBy() != profileRecord.CreatedBy() || restoredProfile.CreatedReason() != profileRecord.CreatedReason() || restoredProfile.PublishedEvidenceRunID() != profileRecord.PublishedEvidenceRunID() || restoredProfile.PublishedReason() != profileRecord.PublishedReason() {
		t.Fatalf("restored Profile = %#v", restoredProfile)
	}

	generationRecord := mapperGeneration(t, profileRecord, now)
	generationPO, err := mapper.GenerationToPO(generationRecord)
	if err != nil {
		t.Fatal(err)
	}
	restoredGeneration, err := mapper.GenerationToDomain(generationPO)
	if err != nil {
		t.Fatal(err)
	}
	if restoredGeneration.Key() != generationRecord.Key() || restoredGeneration.Input().Fingerprint() != generationRecord.Input().Fingerprint() || restoredGeneration.Prompt() != generationRecord.Prompt() {
		t.Fatalf("restored Generation = %#v", restoredGeneration)
	}

	runRecord, err := domainrun.NewPending(meta.FromUint64(800), generationRecord.ID(), 1, retrygovernance.AttemptOriginInitial)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.StartWithLease(now, "trace-1", now.Add(time.Minute), "generation-700/attempt-1"); err != nil {
		t.Fatal(err)
	}
	if err := runRecord.BeginProviderDispatch(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	receipt := aiexplanation.ProviderReceipt{InvocationID: runRecord.InvocationID(), RequestID: "request-1", Provider: "provider-a", Model: "model-a", InputTokens: 10, OutputTokens: 20, Latency: time.Second}
	if err := runRecord.RecordProviderResponse(receipt); err != nil {
		t.Fatal(err)
	}
	if err := runRecord.Succeed(now.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	runPO, err := mapper.RunToPO(runRecord)
	if err != nil {
		t.Fatal(err)
	}
	restoredRun, err := mapper.RunToDomain(runPO)
	if err != nil {
		t.Fatal(err)
	}
	if restoredRun.Status() != domainrun.StatusSucceeded || restoredRun.ProviderReceipt() == nil || restoredRun.ProviderReceipt().RequestID != receipt.RequestID {
		t.Fatalf("restored Run = %#v", restoredRun)
	}

	artifactRecord := mapperArtifact(t, generationRecord, runRecord, profileRecord, receipt, now.Add(2*time.Second))
	artifactPO, err := mapper.ArtifactToPO(artifactRecord)
	if err != nil {
		t.Fatal(err)
	}
	restoredArtifact, err := mapper.ArtifactToDomain(artifactPO)
	if err != nil {
		t.Fatal(err)
	}
	if restoredArtifact.GenerationID() != generationRecord.ID() || restoredArtifact.Content().Summary != artifactRecord.Content().Summary || restoredArtifact.ProviderReceipt().InvocationID != receipt.InvocationID {
		t.Fatalf("restored Artifact = %#v", restoredArtifact)
	}
}

func TestMapperRoundTripsFailedRunRetryAuthorization(t *testing.T) {
	mapper := NewMapper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	runRecord, err := domainrun.NewPending(meta.FromUint64(801), meta.FromUint64(701), 1, retrygovernance.AttemptOriginInitial)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.StartWithLease(now, "trace-1", now.Add(time.Minute), "generation-701/attempt-1"); err != nil {
		t.Fatal(err)
	}
	if err := runRecord.Fail(now.Add(time.Second), domainrun.Failure{Kind: domainrun.FailureKindProviderTimeout, Code: "provider_timeout", SafeMessage: "暂时不可用", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	authorization := domainrun.RetryAuthorization{
		ExpectedAttempt: 1, NextAttempt: 2, Origin: retrygovernance.AttemptOriginManual,
		RequestID: "retry-request-1", EventID: "retry-event-1", Actor: "user:42", Reason: "manual recovery", AuthorizedAt: now.Add(2 * time.Second),
	}
	if err := runRecord.AuthorizeManualRetry(authorization); err != nil {
		t.Fatal(err)
	}
	po, err := mapper.RunToPO(runRecord)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := mapper.RunToDomain(po)
	if err != nil {
		t.Fatal(err)
	}
	stored := restored.RetryAuthorization()
	if stored == nil || !stored.SameAction(authorization) || !stored.AuthorizedAt.Equal(authorization.AuthorizedAt) {
		t.Fatalf("restored retry authorization = %#v", stored)
	}
}

func TestMapperRoundTripsRunningLeaseRecoveryWakeup(t *testing.T) {
	mapper := NewMapper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	runRecord, err := domainrun.NewPending(meta.FromUint64(801), meta.FromUint64(701), 1, retrygovernance.AttemptOriginInitial)
	if err != nil {
		t.Fatal(err)
	}
	leaseExpiresAt := now.Add(time.Minute)
	if err := runRecord.StartWithLease(now, "trace-1", leaseExpiresAt, "generation-701/attempt-1"); err != nil {
		t.Fatal(err)
	}
	wakeup := domainrun.RecoveryWakeup{
		EventID: "lease-recovery-1", ExpectedLeaseExpiresAt: leaseExpiresAt,
		InvocationPhase: domainrun.InvocationPhasePrepared, RequestedAt: leaseExpiresAt.Add(time.Minute),
	}
	if created, err := runRecord.ScheduleRecoveryWakeup(wakeup); err != nil || !created {
		t.Fatalf("schedule wake-up = created:%t error:%v", created, err)
	}
	po, err := mapper.RunToPO(runRecord)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := mapper.RunToDomain(po)
	if err != nil {
		t.Fatal(err)
	}
	stored := restored.RecoveryWakeup()
	if stored == nil || !stored.Same(wakeup) || !stored.RequestedAt.Equal(wakeup.RequestedAt) {
		t.Fatalf("restored recovery wake-up = %#v", stored)
	}
}

func TestMapperRoundTripsApprovedPromptEvaluationEvidence(t *testing.T) {
	mapper := NewMapper()
	createdAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	profileRecord := mapperProfile(t, createdAt.Add(3*time.Hour))
	release := mapperEvaluationRelease(profileRecord)
	runRecord, err := domainevaluation.New(meta.ID(777), release, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	for caseIndex, caseID := range release.GenerationCaseIDs {
		for attempt := 1; attempt <= 5; attempt++ {
			at := createdAt.Add(time.Duration(caseIndex*10+attempt) * time.Minute)
			normalized := []byte(`{"summary":"synthetic"}`)
			receipt := aiexplanation.ProviderReceipt{InvocationID: caseID + "-invocation", RequestID: "request-id", Provider: "provider-a", Model: "model-a", InputTokens: 10, OutputTokens: 20, Latency: time.Second}
			if err := runRecord.AddAttempt(domainevaluation.AttemptRecord{
				CaseID: caseID, Attempt: attempt, Stage: domainevaluation.AttemptStageGeneration, StartedAt: at, FinishedAt: at.Add(time.Second),
				ProviderCallCount: 1, ProviderReceipt: &receipt, RawOutput: normalized, NormalizedOutput: normalized, OutputFingerprint: aiexplanation.NewFingerprint(normalized),
				Assertions: []domainevaluation.AssertionReceipt{
					{Type: "output_schema_valid", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "contract-v1", Status: domainevaluation.AssertionPassed},
					{Type: "case_goal", Scope: domainevaluation.AssertionScopeCase, Ordinal: 1, Evaluator: "semantic-v1", Status: domainevaluation.AssertionPassed},
				},
				Semantic: &domainevaluation.SemanticReceipt{
					EvaluatorVersion: "semantic-rubric-v1",
					ProviderReceipt:  aiexplanation.ProviderReceipt{InvocationID: "semantic-" + caseID, RequestID: "semantic-request", Provider: "judge-provider", Model: "judge-model", Latency: time.Second},
					Rationale:        "reviewed", Scores: domainevaluation.SemanticScores{Faithfulness: 5, CrossDimensionQuality: 5, SuggestionActionability: 5, AudienceClarity: 5, Concision: 5},
				},
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := runRecord.AddAttempt(domainevaluation.AttemptRecord{
		CaseID: "p1", Attempt: 1, Stage: domainevaluation.AttemptStagePreflight, StartedAt: createdAt.Add(75 * time.Minute), FinishedAt: createdAt.Add(75*time.Minute + time.Second),
		ProviderCallCount: 0, RejectionReason: release.PreflightRejectionReason,
		Assertions: []domainevaluation.AssertionReceipt{
			{Type: "provider_call_count", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: domainevaluation.AssertionPassed},
			{Type: "rejection_reason", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: domainevaluation.AssertionPassed},
		},
	}); err != nil {
		t.Fatal(err)
	}
	closedAt := createdAt.Add(90 * time.Minute)
	if err := runRecord.CloseCollection(closedAt); err != nil {
		t.Fatal(err)
	}
	for _, caseID := range release.GenerationCaseIDs {
		for attempt := 1; attempt <= 5; attempt++ {
			for _, role := range []domainevaluation.ReviewRole{domainevaluation.ReviewRoleAssessmentSemantics, domainevaluation.ReviewRoleSafetyProduct} {
				if err := runRecord.AddHumanReview(domainevaluation.HumanReview{CaseID: caseID, Attempt: attempt, Role: role, Reviewer: string(role), Decision: domainevaluation.ReviewDecisionApprove, ReviewedAt: closedAt.Add(time.Minute), Reason: "approved"}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := runRecord.Finalize("release-owner", "all gates passed", closedAt.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	po, err := mapper.PromptEvaluationRunToPO(runRecord)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := mapper.PromptEvaluationRunToDomain(po)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.IsPublishEvidence() || restored.Version() != runRecord.Version() || restored.Release().Suite.Fingerprint != release.Suite.Fingerprint || len(restored.Attempts()) != 36 || len(restored.Reviews()) != 70 {
		t.Fatalf("restored Prompt evaluation = %#v", restored)
	}
}

func TestMapperRoundTripsPromptEvaluationDispatchCheckpoint(t *testing.T) {
	mapper := NewMapper()
	createdAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	profileRecord := mapperProfile(t, createdAt.Add(3*time.Hour))
	release := mapperEvaluationRelease(profileRecord)
	runRecord, err := domainevaluation.New(meta.ID(778), release, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.AddAttempt(domainevaluation.AttemptRecord{
		CaseID: release.PreflightCaseID, Attempt: 1, Stage: domainevaluation.AttemptStagePreflight,
		StartedAt: createdAt.Add(time.Second), FinishedAt: createdAt.Add(2 * time.Second),
		RejectionReason: release.PreflightRejectionReason,
		Assertions: []domainevaluation.AssertionReceipt{
			{Type: "provider_call_count", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: domainevaluation.AssertionPassed},
			{Type: "rejection_reason", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: domainevaluation.AssertionPassed},
		},
	}); err != nil {
		t.Fatal(err)
	}
	claimedAt := createdAt.Add(time.Minute)
	if err := runRecord.BeginAttemptExecution(domainevaluation.AttemptExecution{
		CaseID: "g1", Attempt: 1, Owner: "event-1", InvocationID: "invocation-1",
		Phase: domainevaluation.AttemptExecutionPrepared, ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(3 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if err := runRecord.MarkAttemptDispatching("event-1", claimedAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRecord.RequestRecovery("recovery-1", "user:88", "expired worker delivery", claimedAt.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}

	po, err := mapper.PromptEvaluationRunToPO(runRecord)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := mapper.PromptEvaluationRunToDomain(po)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := restored.Execution()
	if checkpoint == nil || checkpoint.Phase != domainevaluation.AttemptExecutionDispatching || checkpoint.Owner != "event-1" || checkpoint.DispatchStartedAt == nil || len(restored.Recoveries()) != 1 {
		t.Fatalf("restored execution checkpoint = %#v", checkpoint)
	}
}

func TestMapperRoundTripsPromptEvaluationRecheckDispatchCheckpoint(t *testing.T) {
	mapper := NewMapper()
	createdAt := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	release := mapperEvaluationRelease(mapperProfile(t, createdAt))
	value, err := domainevaluation.NewPromptEvaluationRecheck(
		meta.ID(881), meta.ID(778), "g1", 1, release, 12, "user:42", "single record diagnostic", createdAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.BeginDispatch("recheck-event-881", createdAt.Add(time.Second), 5*time.Minute); err != nil {
		t.Fatal(err)
	}
	po, err := mapper.PromptEvaluationRecheckToPO(value)
	if err != nil {
		t.Fatal(err)
	}
	if po.ActiveSourceKey == "" {
		t.Fatal("dispatching recheck must reserve the active source key")
	}
	restored, err := mapper.PromptEvaluationRecheckToDomain(po)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := restored.Execution()
	if restored.Status() != domainevaluation.RecheckStatusDispatching || restored.SourceRunID() != meta.ID(778) ||
		checkpoint == nil || checkpoint.Owner != "recheck-event-881" || checkpoint.Phase != domainevaluation.AttemptExecutionDispatching {
		t.Fatalf("restored recheck = %#v checkpoint=%#v", restored, checkpoint)
	}
}

func TestMapperDerivesActiveOrganizationExecutionKeyFromAuditedCollectingRun(t *testing.T) {
	mapper := NewMapper()
	createdAt := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	runRecord, err := domainevaluation.NewRequested(meta.ID(779), mapperEvaluationRelease(mapperProfile(t, createdAt)), 12, "user:42", "release evaluation", createdAt)
	if err != nil {
		t.Fatal(err)
	}

	po, err := mapper.PromptEvaluationRunToPO(runRecord)
	if err != nil {
		t.Fatal(err)
	}
	if po.ActiveExecutionOrgKey != "12" {
		t.Fatalf("active organization execution key = %q", po.ActiveExecutionOrgKey)
	}
	po.ActiveExecutionOrgKey = "13"
	if _, err := mapper.PromptEvaluationRunToDomain(po); err == nil {
		t.Fatal("expected tampered organization execution key to be rejected")
	}

	if err := runRecord.Cancel("user:42", "stop before dispatch", createdAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	po, err = mapper.PromptEvaluationRunToPO(runRecord)
	if err != nil {
		t.Fatal(err)
	}
	if po.ActiveExecutionOrgKey != "" || po.ActiveReleaseKey != "" {
		t.Fatalf("canceled run retained active keys: organization=%q release=%q", po.ActiveExecutionOrgKey, po.ActiveReleaseKey)
	}
}

func mapperEvaluationRelease(profileRecord *domainprofile.AIExplanationProfile) domainevaluation.ReleaseIdentity {
	return domainevaluation.ReleaseIdentity{
		Suite:             domainevaluation.SuiteRef{ID: "suite-v1", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("suite")), GitBlobSHA: "suite-blob"},
		Prompt:            aiexplanation.PromptRef{TemplateID: profileRecord.Definition().GenerationPolicy.PromptTemplateID, Version: profileRecord.Definition().GenerationPolicy.PromptVersion, Fingerprint: aiexplanation.NewFingerprint([]byte("prompt")), GitBlobSHA: "prompt-blob"},
		Profile:           aiexplanation.ProfileRef{ID: profileRecord.ProfileID(), Version: profileRecord.Version(), Fingerprint: profileRecord.Fingerprint()},
		InputSchema:       domainevaluation.SchemaRef{Version: aiexplanation.InputSchemaVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("input-schema"))},
		OutputSchema:      domainevaluation.SchemaRef{Version: aiexplanation.OutputSchemaVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("output-schema"))},
		Provider:          aiexplanation.ProviderExecutionSpec{Route: profileRecord.Definition().GenerationPolicy.ProviderRoute, RouteRevision: "v1", ResolvedProvider: "provider-a", ResolvedModel: "model-a", Fingerprint: aiexplanation.NewFingerprint([]byte("provider-route"))},
		Decoding:          domainevaluation.DecodingParameters{MaxOutputTokens: 3000, ReasoningEffort: "low"},
		SemanticEvaluator: mapperSemanticEvaluator(),
		GenerationCaseIDs: []string{"g1", "g2", "g3", "g4", "g5", "g6", "g7"}, PreflightCaseID: "p1",
		PreflightRejectionReason: "insufficient_eligible_dimensions", RepetitionsPerCase: 5,
	}
}

func mapperSemanticEvaluator() domainevaluation.SemanticEvaluatorSpec {
	return domainevaluation.SemanticEvaluatorSpec{
		Version: "semantic-rubric-v1",
		Prompt: aiexplanation.PromptRef{
			TemplateID: "ai-explanation-semantic-evaluator", Version: "v1",
			Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-prompt")), GitBlobSHA: "semantic-prompt-blob",
		},
		OutputSchema: domainevaluation.SchemaRef{Version: "ai-explanation-semantic-evaluation-output/v1", Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-schema"))},
		Provider: aiexplanation.ProviderExecutionSpec{
			Route: "semantic_judge_v1", RouteRevision: "v1", ResolvedProvider: "judge-provider", ResolvedModel: "judge-model",
			Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-route")),
		},
		Decoding: domainevaluation.DecodingParameters{MaxOutputTokens: 2000, ReasoningEffort: "low"},
	}
}

func TestEvaluationAttemptMapperRoundTripsFailureAndAssertionOrdinal(t *testing.T) {
	at := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	attempt := domainevaluation.AttemptRecord{
		CaseID: "g1", Attempt: 1, Stage: domainevaluation.AttemptStageGeneration,
		StartedAt: at, FinishedAt: at.Add(time.Second), ProviderCallCount: 1,
		Failure: &domainevaluation.AttemptFailure{
			Stage: "provider_execution", Code: "provider_timeout", SafeMessage: "Provider timed out", Retryable: true, ResultUnknown: true,
		},
		Assertions: []domainevaluation.AssertionReceipt{{
			Type: "forbid_dimension_group", Scope: domainevaluation.AssertionScopeCase, Ordinal: 2,
			Hard: true, Evaluator: "runner-v1", Status: domainevaluation.AssertionFailed, Detail: "provider_timeout",
		}},
	}
	po := attemptToPO(attempt)
	restored, err := attemptFromPO(po)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Failure == nil || *restored.Failure != *attempt.Failure || len(restored.Assertions) != 1 || restored.Assertions[0].Ordinal != 2 {
		t.Fatalf("restored attempt = %#v", restored)
	}
	if err := restored.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluationAttemptMapperRoundTripsSemanticFailureEvidenceThroughBSON(t *testing.T) {
	at := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	normalized := []byte(`{"summary":"candidate"}`)
	generationReceipt := aiexplanation.ProviderReceipt{
		InvocationID: "generation:g1:1", RequestID: "generation-request", Provider: "provider-a", Model: "model-a", Latency: time.Second,
	}
	semanticReceipt := aiexplanation.ProviderReceipt{
		InvocationID: "semantic:g1:1", RequestID: "semantic-request", Provider: "judge-provider", Model: "judge-model", Latency: time.Second,
	}
	failure := domainevaluation.AttemptFailure{
		Stage: string(domainevaluation.FailureStageSemanticEvaluation),
		Code:  domainevaluation.SemanticOutputSchemaInvalid, SafeMessage: "semantic output violated the frozen schema", Retryable: true,
	}
	attempt := domainevaluation.AttemptRecord{
		CaseID: "g1", Attempt: 1, Stage: domainevaluation.AttemptStageGeneration,
		StartedAt: at, FinishedAt: at.Add(2 * time.Second), ProviderCallCount: 1,
		ProviderReceipt: &generationReceipt, RawOutput: normalized, NormalizedOutput: normalized,
		OutputFingerprint: aiexplanation.NewFingerprint(normalized), Failure: &failure,
		Assertions: []domainevaluation.AssertionReceipt{
			{Type: "output_schema_valid", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "contract-v1", Status: domainevaluation.AssertionPassed},
			{Type: string(domainevaluation.FailureStageSemanticEvaluation), Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "runner-v1", Status: domainevaluation.AssertionFailed, Detail: failure.Code},
		},
		SemanticExecution: &domainevaluation.SemanticExecutionRecord{
			InvocationID: semanticReceipt.InvocationID, EvaluatorVersion: "semantic-rubric-v1",
			StartedAt: at.Add(time.Second), FinishedAt: at.Add(2 * time.Second), ProviderCallCount: 1,
			ProviderReceipt: &semanticReceipt, RawOutput: []byte(`{"unexpected":true}`),
			NormalizedOutput: []byte(`{"unexpected":true}`), Failure: &failure,
		},
	}
	if err := attempt.Validate(); err != nil {
		t.Fatal(err)
	}
	payload, err := bson.Marshal(attemptToPO(attempt))
	if err != nil {
		t.Fatal(err)
	}
	var persisted EvaluationAttemptPO
	if err := bson.Unmarshal(payload, &persisted); err != nil {
		t.Fatal(err)
	}
	restored, err := attemptFromPO(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if restored.SemanticExecution == nil || restored.SemanticExecution.Failure == nil ||
		restored.SemanticExecution.Failure.Code != failure.Code || restored.SemanticExecution.ProviderReceipt == nil ||
		string(restored.SemanticExecution.RawOutput) != `{"unexpected":true}` {
		t.Fatalf("restored semantic execution = %#v", restored.SemanticExecution)
	}
	if err := restored.Validate(); err != nil {
		t.Fatal(err)
	}
}

func mapperProfile(t *testing.T, now time.Time) *domainprofile.AIExplanationProfile {
	t.Helper()
	definition := domainprofile.Definition{
		SchemaVersion: aiexplanation.ProfileSchemaVersionV1, ProfileID: "participant-scale", Version: "v1",
		Selector:         domainprofile.Selector{Audience: policy.AudienceParticipant, ModelKind: modelcatalog.KindScale, DecisionKind: modelcatalog.DecisionKindScoreRange},
		Eligibility:      domainprofile.EligibilityPolicy{MinEligibleDimensions: 2, MaxInputDimensions: 12, OnDimensionOverflow: "reject"},
		InputPolicy:      domainprofile.InputPolicy{ContextScope: "current_assessment_only"},
		InsightPolicy:    domainprofile.InsightPolicy{AllowedKinds: []output.InsightKind{output.InsightKindReinforcingPattern}, MinItems: 1, MaxItems: 2, MinDimensionRefsPerItem: 2, MaxDimensionRefsPerItem: 3},
		SuggestionPolicy: domainprofile.SuggestionPolicy{AllowedOrigins: []output.SuggestionOrigin{output.SuggestionOriginStandardDerived}, AllowedCategories: []string{"daily_practice"}, MinItems: 1, MaxItems: 2, MaxActionsPerItem: 2, RequireEvidenceRefs: true, RequireStandardRefsForStandardDerived: true},
		SafetyPolicy:     domainprofile.SafetyPolicy{PolicyVersion: "v1", DisclaimerVersion: "v1", ForbiddenClaims: []string{"diagnosis", "causality", "medication", "treatment_plan", "risk_reclassification", "identity_inference", "deterministic_future_prediction"}},
		GenerationPolicy: domainprofile.GenerationPolicy{PromptTemplateID: "cross-dimension-participant-scale", PromptVersion: "v1", ProviderRoute: "balanced_text_v1", InputSchemaVersion: aiexplanation.InputSchemaVersionV1, OutputSchemaVersion: aiexplanation.OutputSchemaVersionV1, MaxOutputCharacters: 8000},
	}
	profileRecord, err := domainprofile.NewDraftForRelease(meta.FromUint64(500), definition, "user:41", "initial release candidate", now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := profileRecord.Publish(meta.ID(101), "tester", "approved evaluation", now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	return profileRecord
}

func mapperGeneration(t *testing.T, profileRecord *domainprofile.AIExplanationProfile, now time.Time) *domaingeneration.AIExplanationGeneration {
	t.Helper()
	snapshot, err := domaininput.NewSnapshot([]byte(`{"schema_version":"ai-explanation-input/v1"}`))
	if err != nil {
		t.Fatal(err)
	}
	profileRef := aiexplanation.ProfileRef{ID: profileRecord.ProfileID(), Version: profileRecord.Version(), Fingerprint: profileRecord.Fingerprint()}
	execution := aiexplanation.ProviderExecutionSpec{Route: "balanced_text_v1", RouteRevision: "v1", ResolvedProvider: "provider-a", ResolvedModel: "model-a", Fingerprint: aiexplanation.NewFingerprint([]byte("route"))}
	generationRecord, err := domaingeneration.New(domaingeneration.NewInput{
		ID: meta.FromUint64(700), Key: domaingeneration.Key{SourceReportID: meta.FromUint64(101), Audience: policy.AudienceParticipant, Profile: profileRef, InputFingerprint: snapshot.Fingerprint(), ExecutionSpecFingerprint: execution.Fingerprint},
		Association: aiexplanation.Association{OrgID: 1, AssessmentID: meta.FromUint64(7), TesteeID: 9}, RequestedBy: aiexplanation.ActorRef{Kind: "participant", ID: "user-1"}, Input: snapshot,
		Prompt: aiexplanation.PromptRef{TemplateID: "cross-dimension-participant-scale", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("prompt")), GitBlobSHA: "abc123"}, ExecutionSpec: execution, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return generationRecord
}

func mapperArtifact(t *testing.T, generationRecord *domaingeneration.AIExplanationGeneration, runRecord *domainrun.AIExplanationRun, profileRecord *domainprofile.AIExplanationProfile, receipt aiexplanation.ProviderReceipt, at time.Time) *domainartifact.AIExplanationArtifact {
	t.Helper()
	content := output.Content{
		SchemaVersion: aiexplanation.OutputSchemaVersionV1, Summary: "睡眠与压力可以结合观察。",
		IntegratedInsights: []output.IntegratedInsight{{Kind: output.InsightKindReinforcingPattern, Title: "组合关注", Content: "两个维度可结合观察。", WhyItMatters: "有助于理解本次结果。", EvidenceRefs: []output.EvidenceRef{{Kind: output.EvidenceKindDimension, Ref: "dimension:sleep"}, {Kind: output.EvidenceKindDimension, Ref: "dimension:stress"}}}},
		Suggestions:        []output.Suggestion{{Origin: output.SuggestionOriginStandardDerived, Category: "daily_practice", Title: "记录节律", Goal: "观察变化", Actions: []string{"每天记录一次"}, Rationale: "来自标准建议。", EvidenceRefs: []output.EvidenceRef{{Kind: output.EvidenceKindStandardSuggestion, Ref: "suggestion:sleep-note"}}, SourceSuggestionRefs: []string{"suggestion:sleep-note"}}},
		Limitations:        []string{"仅基于本次测评，不构成诊断或确定性判断。"},
	}
	artifactRecord, err := domainartifact.New(domainartifact.NewInput{
		ID: meta.FromUint64(900), GenerationID: generationRecord.ID(), RunID: runRecord.ID(), Source: domainartifact.SourceRef{ReportID: meta.FromUint64(101), OutcomeID: meta.FromUint64(301), Association: generationRecord.Association(), ReportType: "standard", TemplateVersion: "v1", ContentSchemaVersion: "v1", BuilderIdentity: "factor-scoring", ReportGeneratedAt: at.Add(-time.Hour)},
		Audience: policy.AudienceParticipant, Profile: generationRecord.Key().Profile, Prompt: generationRecord.Prompt(), ExecutionSpec: generationRecord.ExecutionSpec(), InputSchema: aiexplanation.InputSchemaVersionV1, InputFingerprint: generationRecord.Input().Fingerprint(), OutputSchema: aiexplanation.OutputSchemaVersionV1,
		SafetyPolicy: profileRecord.Definition().SafetyPolicy.PolicyVersion, ProviderReceipt: receipt, Validation: domainartifact.ValidationReceipt{SchemaValidatorVersion: "schema/v1", ReferenceValidatorVersion: "reference/v1", ProfileValidatorVersion: "profile/v1", SafetyValidatorVersion: "safety/v1", ValidatedAt: at}, Content: content, GeneratedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifactRecord
}
