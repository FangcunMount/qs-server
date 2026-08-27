//go:build integration

package aiexplanation_test

import (
	"errors"
	"testing"
	"time"

	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	aievents "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/events"
	mongoeventoutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventoutbox"
	mongoai "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation/aiexplanation"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
)

func TestAIExplanationPromptEvaluationProgressIsAtomicOnReplicaSet(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	retention := mongoai.RetentionPolicy{
		Version: "integration-v1", ParticipantRecordRetention: 24 * time.Hour,
		PromptEvaluationRetention: 24 * time.Hour, CapacityLedgerRetention: 24 * time.Hour,
	}
	repository, err := mongoai.NewPromptEvaluationRepository(db, retention)
	if err != nil {
		t.Fatal(err)
	}
	capacity, err := mongoai.NewPromptEvaluationBudgetRepository(db, retention)
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := mongoeventoutbox.NewStoreWithTopicResolver(db, aiExplanationTopicResolver{})
	if err != nil {
		t.Fatal(err)
	}
	runner := aiExplanationMongoRunner(db)
	now := time.Date(2026, 8, 27, 14, 0, 0, 0, time.UTC)
	newCommitter := func(t *testing.T, stager appevaluation.PromptEvaluationEventStager) *appevaluation.DurableCommitter {
		t.Helper()
		committer, err := appevaluation.NewDurableCommitter(
			runner, repository, aievents.Factory{}, stager, noopAIPostCommit{}, capacity,
			appevaluation.MaxProviderInvocationsV1*2, func() time.Time { return now },
		)
		if err != nil {
			t.Fatal(err)
		}
		return committer
	}
	runRecord := integrationPromptEvaluationRun(t, now, "primary")

	t.Run("start rolls back full budget reservation and run when first step staging fails", func(t *testing.T) {
		committer := newCommitter(t, failingAIEventStager{err: errors.New("injected evaluation outbox failure")})
		if err := committer.CommitStart(t.Context(), runRecord); err == nil {
			t.Fatal("CommitStart() error = nil, want injected failure")
		}
		assertMongoCount(t, db.Collection("ai_explanation_prompt_evaluation_runs"), bson.M{"domain_id": runRecord.ID()}, 0)
		assertMongoCount(t, db.Collection("domain_event_outbox"), bson.M{"aggregate_id": runRecord.ID().String()}, 0)
		usage, found, findErr := capacity.FindDailyCapacityUsage(t.Context(), 81, domainevaluation.UTCBudgetDay(now))
		if findErr != nil || !found || usage.ReservedProviderInvocations != 0 || len(usage.Reservations) != 0 {
			t.Fatalf("rolled-back evaluation capacity = %#v, %v, %v", usage, found, findErr)
		}
	})

	committer := newCommitter(t, outbox)
	if err := committer.CommitStart(t.Context(), runRecord); err != nil {
		t.Fatalf("CommitStart() error = %v", err)
	}
	persisted, err := repository.FindByID(t.Context(), runRecord.ID())
	if err != nil || persisted.Status() != domainevaluation.StatusCollecting || persisted.HasAttempt("g1", 1) {
		t.Fatalf("persisted evaluation start = %#v, %v", persisted, err)
	}
	usage, found, err := capacity.FindDailyCapacityUsage(t.Context(), 81, domainevaluation.UTCBudgetDay(now))
	if err != nil || !found || usage.ReservedProviderInvocations != appevaluation.MaxProviderInvocationsV1 || len(usage.Reservations) != 1 {
		t.Fatalf("committed evaluation capacity = %#v, %v, %v", usage, found, err)
	}
	assertMongoCount(t, db.Collection("domain_event_outbox"), bson.M{
		"aggregate_id": runRecord.ID().String(), "event_type": eventcatalog.AIExplanationPromptEvaluationStepRequested,
	}, 1)

	evidence, err := appevaluation.NewEvidenceService(repository, meta.New, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := evidence.ClaimAttempt(t.Context(), runRecord.ID(), appevaluation.ClaimAttemptCommand{
		CaseID: "g1", Attempt: 1, Owner: "event-g1-1", InvocationID: "evaluation-invocation-g1-1",
		ClaimedAt: now.Add(time.Second), LeaseExpiresAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := evidence.MarkAttemptDispatching(t.Context(), runRecord.ID(), "event-g1-1", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	attempt := integrationPromptEvaluationAttempt(now)

	t.Run("completed attempt rolls back checkpoint and next step when staging fails", func(t *testing.T) {
		committer := newCommitter(t, failingAIEventStager{err: errors.New("injected continuation outbox failure")})
		if _, err := committer.CommitAttempt(t.Context(), runRecord.ID(), "event-g1-1", attempt); err == nil {
			t.Fatal("CommitAttempt() error = nil, want injected failure")
		}
		stored, findErr := repository.FindByID(t.Context(), runRecord.ID())
		if findErr != nil || stored.HasAttempt("g1", 1) || stored.Execution() == nil ||
			stored.Execution().Phase != domainevaluation.AttemptExecutionDispatching {
			t.Fatalf("rolled-back evaluation checkpoint = %#v, %v", stored, findErr)
		}
		assertMongoCount(t, db.Collection("domain_event_outbox"), bson.M{
			"aggregate_id": runRecord.ID().String(), "event_type": eventcatalog.AIExplanationPromptEvaluationStepRequested,
		}, 1)
	})

	updated, err := committer.CommitAttempt(t.Context(), runRecord.ID(), "event-g1-1", attempt)
	if err != nil {
		t.Fatalf("CommitAttempt() error = %v", err)
	}
	if !updated.HasAttempt("g1", 1) || updated.Execution() != nil {
		t.Fatalf("committed evaluation checkpoint = %#v", updated)
	}
	assertMongoCount(t, db.Collection("domain_event_outbox"), bson.M{
		"aggregate_id": runRecord.ID().String(), "event_type": eventcatalog.AIExplanationPromptEvaluationStepRequested,
	}, 2)

	t.Run("second active run for the same organization rolls back its reservation", func(t *testing.T) {
		second := integrationPromptEvaluationRun(t, now, "secondary")
		err := committer.CommitStart(t.Context(), second)
		if !errors.Is(err, domainevaluation.ErrOrgConcurrencyExceeded) {
			t.Fatalf("second organization run error = %v", err)
		}
		usage, found, findErr := capacity.FindDailyCapacityUsage(t.Context(), 81, domainevaluation.UTCBudgetDay(now))
		if findErr != nil || !found || usage.ReservedProviderInvocations != appevaluation.MaxProviderInvocationsV1 || len(usage.Reservations) != 1 {
			t.Fatalf("organization concurrency rollback capacity = %#v, %v, %v", usage, found, findErr)
		}
		assertMongoCount(t, db.Collection("ai_explanation_prompt_evaluation_runs"), bson.M{"domain_id": second.ID()}, 0)
	})
}

func integrationPromptEvaluationRun(t *testing.T, now time.Time, releaseSalt string) *domainevaluation.PromptEvaluationRun {
	t.Helper()
	runRecord, err := domainevaluation.NewRequested(meta.New(), integrationEvaluationRelease(releaseSalt), 81, "integration-operator", "release evaluation", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.AddAttempt(domainevaluation.AttemptRecord{
		CaseID: "preflight", Attempt: 1, Stage: domainevaluation.AttemptStagePreflight,
		StartedAt: now, FinishedAt: now, RejectionReason: "insufficient_eligible_dimensions",
		Assertions: []domainevaluation.AssertionReceipt{{
			Type: "provider_call_count", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1,
			Hard: true, Evaluator: "preflight-v1", Status: domainevaluation.AssertionPassed,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return runRecord
}

func integrationEvaluationRelease(salt string) domainevaluation.ReleaseIdentity {
	return domainevaluation.ReleaseIdentity{
		Suite: domainevaluation.SuiteRef{
			ID: "participant-scale-v1-" + salt, Version: "v1",
			Fingerprint: aiexplanation.NewFingerprint([]byte("suite-" + salt)), GitBlobSHA: "suite-blob-" + salt,
		},
		Prompt: aiexplanation.PromptRef{
			TemplateID: "ai-explanation-" + salt, Version: "v1",
			Fingerprint: aiexplanation.NewFingerprint([]byte("prompt-" + salt)), GitBlobSHA: "prompt-blob-" + salt,
		},
		Profile: aiexplanation.ProfileRef{
			ID: "participant-scale-" + salt, Version: "v1",
			Fingerprint: aiexplanation.NewFingerprint([]byte("profile-" + salt)),
		},
		InputSchema: domainevaluation.SchemaRef{
			Version: aiexplanation.InputSchemaVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("input-schema")),
		},
		OutputSchema: domainevaluation.SchemaRef{
			Version: aiexplanation.OutputSchemaVersionV1, Fingerprint: aiexplanation.NewFingerprint([]byte("output-schema")),
		},
		Provider: aiexplanation.ProviderExecutionSpec{
			Route: "balanced_text_v1", RouteRevision: "v1", ResolvedProvider: "provider-a",
			ResolvedModel: "model-a", Fingerprint: aiexplanation.NewFingerprint([]byte("route")),
		},
		Decoding: domainevaluation.DecodingParameters{MaxOutputTokens: 3000},
		SemanticEvaluator: domainevaluation.SemanticEvaluatorSpec{
			Version: "semantic-rubric-v1",
			Prompt: aiexplanation.PromptRef{
				TemplateID: "semantic-evaluator", Version: "v1",
				Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-prompt")), GitBlobSHA: "semantic-blob",
			},
			OutputSchema: domainevaluation.SchemaRef{
				Version:     aiexplanation.SemanticEvaluationOutputSchemaVersionV1,
				Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-schema")),
			},
			Provider: aiexplanation.ProviderExecutionSpec{
				Route: "semantic_judge_v1", RouteRevision: "v1", ResolvedProvider: "judge-provider",
				ResolvedModel: "judge-model", Fingerprint: aiexplanation.NewFingerprint([]byte("semantic-route")),
			},
			Decoding: domainevaluation.DecodingParameters{MaxOutputTokens: 2000},
		},
		GenerationCaseIDs: []string{"g1", "g2", "g3", "g4", "g5", "g6", "g7"},
		PreflightCaseID:   "preflight", PreflightRejectionReason: "insufficient_eligible_dimensions",
		RepetitionsPerCase: domainevaluation.RequiredRepetitionsPerCase,
	}
}

func integrationPromptEvaluationAttempt(now time.Time) domainevaluation.AttemptRecord {
	normalized := []byte(`{"summary":"synthetic"}`)
	receipt := aiexplanation.ProviderReceipt{
		InvocationID: "evaluation-invocation-g1-1", RequestID: "provider-request-g1-1",
		Provider: "provider-a", Model: "model-a", InputTokens: 100, OutputTokens: 20, Latency: time.Second,
	}
	return domainevaluation.AttemptRecord{
		CaseID: "g1", Attempt: 1, Stage: domainevaluation.AttemptStageGeneration,
		StartedAt: now.Add(2 * time.Second), FinishedAt: now.Add(3 * time.Second), ProviderCallCount: 1,
		ProviderReceipt: &receipt, RawOutput: normalized, NormalizedOutput: normalized,
		OutputFingerprint: aiexplanation.NewFingerprint(normalized),
		Assertions: []domainevaluation.AssertionReceipt{{
			Type: "output_schema_valid", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1,
			Hard: true, Evaluator: "contract-v1", Status: domainevaluation.AssertionPassed,
		}},
	}
}

var _ appevaluation.PromptEvaluationEventStager = (*mongoeventoutbox.Store)(nil)
var _ appevaluation.PromptEvaluationEventStager = failingAIEventStager{}
