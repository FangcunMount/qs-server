//go:build integration

package aiexplanation_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	aipersistence "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/persistence"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	aievents "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/events"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domaininput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/input"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	mongoeventoutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventoutbox"
	mongoai "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation/aiexplanation"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

func TestAIExplanationParticipantLifecycleIsAtomicOnReplicaSet(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	fixture := newAIExplanationMongoFixture(t, db)
	now := time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC)
	generation := integrationGeneration(t, now)

	t.Run("request rolls back budget reservation and generation when outbox staging fails", func(t *testing.T) {
		committer := fixture.committer(t, failingAIEventStager{err: errors.New("injected outbox failure")})
		if err := committer.CommitRequested(t.Context(), generation); err == nil {
			t.Fatal("CommitRequested() error = nil, want injected failure")
		}

		assertMongoCount(t, db.Collection("ai_explanation_generations"), bson.M{"domain_id": generation.ID()}, 0)
		assertMongoCount(t, db.Collection("domain_event_outbox"), bson.M{"aggregate_id": generation.ID().String()}, 0)
		usage, found, err := fixture.budget.FindParticipantDailyCapacityUsage(t.Context(), 71, domaingeneration.ParticipantUTCBudgetDay(now))
		if err != nil || !found {
			t.Fatalf("FindParticipantDailyCapacityUsage() = %#v, %v, %v", usage, found, err)
		}
		if usage.ReservedProviderInvocations != 0 || len(usage.Reservations) != 0 {
			t.Fatalf("rolled-back request capacity = %#v", usage)
		}
	})

	committer := fixture.committer(t, fixture.outbox)
	if err := committer.CommitRequested(t.Context(), generation); err != nil {
		t.Fatalf("CommitRequested() error = %v", err)
	}
	persistedGeneration, err := fixture.generations.FindByID(t.Context(), generation.ID())
	if err != nil {
		t.Fatal(err)
	}
	if persistedGeneration.Status() != domaingeneration.StatusPending ||
		!bytes.Equal(persistedGeneration.Input().CanonicalJSON(), generation.Input().CanonicalJSON()) {
		t.Fatalf("persisted requested Generation = %#v", persistedGeneration)
	}
	var rawGeneration bson.M
	if err := db.Collection("ai_explanation_generations").FindOne(t.Context(), bson.M{"domain_id": generation.ID()}).Decode(&rawGeneration); err != nil {
		t.Fatal(err)
	}
	if _, exists := rawGeneration["input_json"]; !exists {
		t.Fatalf("Generation input snapshot is missing: %#v", rawGeneration)
	}
	usage, found, err := fixture.budget.FindParticipantDailyCapacityUsage(t.Context(), 71, domaingeneration.ParticipantUTCBudgetDay(now))
	if err != nil || !found || usage.ReservedProviderInvocations != 1 || len(usage.Reservations) != 1 {
		t.Fatalf("committed request capacity = %#v, %v, %v", usage, found, err)
	}
	assertMongoCount(t, db.Collection("domain_event_outbox"), bson.M{
		"aggregate_id": generation.ID().String(), "event_type": eventcatalog.AIExplanationRequested,
	}, 1)

	runRecord, err := domainrun.NewPending(meta.New(), generation.ID(), 1, retrygovernance.AttemptOriginInitial)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.StartWithLease(now.Add(time.Second), "integration-trace", now.Add(time.Minute), "integration-invocation"); err != nil {
		t.Fatal(err)
	}
	expectedVersion := generation.Version()
	if err := generation.Begin(runRecord.ID(), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	t.Run("start rolls back active slot when run creation fails", func(t *testing.T) {
		committer := fixture.committerWithRun(t, failingAIRunRepository{
			Repository: fixture.runs, err: errors.New("injected run failure"),
		}, fixture.outbox)
		if err := committer.CommitStart(t.Context(), generation, runRecord, expectedVersion); err == nil {
			t.Fatal("CommitStart() error = nil, want injected failure")
		}
		stored, findErr := fixture.generations.FindByID(t.Context(), generation.ID())
		if findErr != nil || stored.Status() != domaingeneration.StatusPending {
			t.Fatalf("rolled-back Generation = %#v, %v", stored, findErr)
		}
		if _, findErr := fixture.runs.FindByID(t.Context(), runRecord.ID()); !errors.Is(findErr, domainrun.ErrNotFound) {
			t.Fatalf("rolled-back Run lookup error = %v", findErr)
		}
		active, found, findErr := fixture.active.FindParticipantActiveCapacityUsage(t.Context(), 71)
		if findErr != nil || !found || active.ActiveExecutions != 0 || len(active.Reservations) != 0 {
			t.Fatalf("rolled-back active capacity = %#v, %v, %v", active, found, findErr)
		}
	})

	if err := committer.CommitStart(t.Context(), generation, runRecord, expectedVersion); err != nil {
		t.Fatalf("CommitStart() error = %v", err)
	}
	if err := runRecord.BeginProviderDispatch(now.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := committer.SaveDispatching(t.Context(), runRecord); err != nil {
		t.Fatal(err)
	}
	expectedVersion = generation.Version()
	failure := domainrun.Failure{
		Kind: domainrun.FailureKindProviderTimeout, Code: "provider_timeout",
		SafeMessage: "AI 解读暂时不可用", Retryable: true,
	}
	if err := runRecord.Fail(now.Add(3*time.Second), failure); err != nil {
		t.Fatal(err)
	}
	if err := generation.Fail(runRecord.ID(), now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}

	t.Run("terminal failure rolls back state and slot release when outbox staging fails", func(t *testing.T) {
		committer := fixture.committer(t, failingAIEventStager{err: errors.New("injected terminal outbox failure")})
		if err := committer.CommitFailure(t.Context(), generation, runRecord, expectedVersion); err == nil {
			t.Fatal("CommitFailure() error = nil, want injected failure")
		}
		storedGeneration, findErr := fixture.generations.FindByID(t.Context(), generation.ID())
		if findErr != nil || storedGeneration.Status() != domaingeneration.StatusGenerating {
			t.Fatalf("rolled-back terminal Generation = %#v, %v", storedGeneration, findErr)
		}
		storedRun, findErr := fixture.runs.FindByID(t.Context(), runRecord.ID())
		if findErr != nil || storedRun.Status() != domainrun.StatusRunning || storedRun.InvocationPhase() != domainrun.InvocationPhaseDispatching {
			t.Fatalf("rolled-back terminal Run = %#v, %v", storedRun, findErr)
		}
		active, found, findErr := fixture.active.FindParticipantActiveCapacityUsage(t.Context(), 71)
		if findErr != nil || !found || active.ActiveExecutions != 1 || len(active.Reservations) != 1 {
			t.Fatalf("rolled-back terminal active capacity = %#v, %v, %v", active, found, findErr)
		}
		assertMongoCount(t, db.Collection("domain_event_outbox"), bson.M{
			"aggregate_id": generation.ID().String(), "event_type": eventcatalog.AIExplanationFailed,
		}, 0)
	})

	if err := committer.CommitFailure(t.Context(), generation, runRecord, expectedVersion); err != nil {
		t.Fatalf("CommitFailure() error = %v", err)
	}
	storedGeneration, err := fixture.generations.FindByID(t.Context(), generation.ID())
	if err != nil || storedGeneration.Status() != domaingeneration.StatusFailed {
		t.Fatalf("committed failed Generation = %#v, %v", storedGeneration, err)
	}
	storedRun, err := fixture.runs.FindByID(t.Context(), runRecord.ID())
	if err != nil || storedRun.Status() != domainrun.StatusFailed {
		t.Fatalf("committed failed Run = %#v, %v", storedRun, err)
	}
	active, found, err := fixture.active.FindParticipantActiveCapacityUsage(t.Context(), 71)
	if err != nil || !found || active.ActiveExecutions != 0 || len(active.Reservations) != 0 {
		t.Fatalf("released active capacity = %#v, %v, %v", active, found, err)
	}
	assertMongoCount(t, db.Collection("domain_event_outbox"), bson.M{
		"aggregate_id": generation.ID().String(), "event_type": eventcatalog.AIExplanationFailed,
	}, 1)
}

type aiExplanationMongoFixture struct {
	db          *mongo.Database
	runner      apptransaction.Runner
	generations *mongoai.GenerationRepository
	runs        *mongoai.RunRepository
	artifacts   *mongoai.ArtifactRepository
	budget      *mongoai.ParticipantBudgetRepository
	active      *mongoai.ParticipantActiveCapacityRepository
	outbox      *mongoeventoutbox.Store
	retention   mongoai.RetentionPolicy
	policy      domaingeneration.ParticipantCapacityPolicy
}

func newAIExplanationMongoFixture(t *testing.T, db *mongo.Database) aiExplanationMongoFixture {
	t.Helper()
	retention := mongoai.RetentionPolicy{
		Version: "integration-v1", ParticipantRecordRetention: 24 * time.Hour,
		PromptEvaluationRetention: 24 * time.Hour, CapacityLedgerRetention: 24 * time.Hour,
	}
	generations, err := mongoai.NewGenerationRepository(db, retention)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := mongoai.NewRunRepository(db, retention)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := mongoai.NewArtifactRepository(db, retention)
	if err != nil {
		t.Fatal(err)
	}
	budget, err := mongoai.NewParticipantBudgetRepository(db, retention)
	if err != nil {
		t.Fatal(err)
	}
	active, err := mongoai.NewParticipantActiveCapacityRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := mongoeventoutbox.NewStoreWithTopicResolver(db, aiExplanationTopicResolver{})
	if err != nil {
		t.Fatal(err)
	}
	return aiExplanationMongoFixture{
		db: db, runner: aiExplanationMongoRunner(db), generations: generations, runs: runs,
		artifacts: artifacts, budget: budget, active: active, outbox: outbox, retention: retention,
		policy: domaingeneration.ParticipantCapacityPolicy{
			DailyProviderInvocationBudgetPerOrg: 500, DailyProviderInvocationBudgetPerUser: 5,
			DailyProviderInvocationBudgetPerAssessment: 3, MaxActiveProviderExecutionsPerOrg: 10,
			MaxActiveProviderExecutionsPerUser: 2, MaxActiveProviderExecutionsPerAssessment: 1,
		},
	}
}

func (f aiExplanationMongoFixture) committer(t *testing.T, stager aipersistence.EventStager) *aipersistence.Committer {
	t.Helper()
	return f.committerWithRun(t, f.runs, stager)
}

func (f aiExplanationMongoFixture) committerWithRun(t *testing.T, runs domainrun.Repository, stager aipersistence.EventStager) *aipersistence.Committer {
	t.Helper()
	committer, err := aipersistence.NewCommitter(
		f.runner, f.generations, runs, f.artifacts, f.budget, f.active, f.policy,
		aievents.Factory{}, stager, noopAIPostCommit{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return committer
}

type failingAIRunRepository struct {
	domainrun.Repository
	err error
}

func (r failingAIRunRepository) Create(context.Context, *domainrun.AIExplanationRun) error {
	return r.err
}

type failingAIEventStager struct{ err error }

func (s failingAIEventStager) Stage(context.Context, ...event.DomainEvent) error { return s.err }

type noopAIPostCommit struct{}

func (noopAIPostCommit) AfterCommit(context.Context, []event.DomainEvent, time.Time) {}

type aiExplanationTopicResolver struct{}

func (aiExplanationTopicResolver) GetTopicForEvent(string) (string, bool) {
	return "integration.ai-explanation", true
}

func aiExplanationMongoRunner(db *mongo.Database) apptransaction.Runner {
	return apptransaction.RunnerFunc(func(ctx context.Context, callback func(context.Context) error) error {
		session, err := db.Client().StartSession()
		if err != nil {
			return err
		}
		defer session.EndSession(ctx)
		transactionOptions := options.Transaction().
			SetReadConcern(readconcern.Snapshot()).
			SetWriteConcern(writeconcern.Majority()).
			SetReadPreference(readpref.Primary())
		_, err = session.WithTransaction(ctx, func(sessionContext mongo.SessionContext) (any, error) {
			return nil, callback(sessionContext)
		}, transactionOptions)
		return err
	})
}

func integrationGeneration(t *testing.T, now time.Time) *domaingeneration.AIExplanationGeneration {
	t.Helper()
	snapshot, err := domaininput.NewSnapshot([]byte(`{"schema_version":"ai-explanation-input/v1","sensitive":"integration-only"}`))
	if err != nil {
		t.Fatal(err)
	}
	profile := aiexplanation.ProfileRef{
		ID: "participant-scale", Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte("integration-profile")),
	}
	execution := aiexplanation.ProviderExecutionSpec{
		Route: "balanced_text_v1", RouteRevision: "v1", ResolvedProvider: "provider-a",
		ResolvedModel: "model-a", Fingerprint: aiexplanation.NewFingerprint([]byte("integration-route")),
	}
	generation, err := domaingeneration.New(domaingeneration.NewInput{
		ID: meta.New(),
		Key: domaingeneration.Key{
			SourceReportID: meta.New(), Audience: policy.AudienceParticipant, Profile: profile,
			InputFingerprint: snapshot.Fingerprint(), ExecutionSpecFingerprint: execution.Fingerprint,
		},
		Association: aiexplanation.Association{OrgID: 71, AssessmentID: meta.New(), TesteeID: 902},
		RequestedBy: aiexplanation.ActorRef{Kind: "participant", ID: "integration-user"},
		Input:       snapshot,
		Prompt: aiexplanation.PromptRef{
			TemplateID: "cross-dimension-participant-scale", Version: "v1",
			Fingerprint: aiexplanation.NewFingerprint([]byte("integration-prompt")), GitBlobSHA: "integration-blob",
		},
		ExecutionSpec: execution, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return generation
}

func assertMongoCount(t *testing.T, collection *mongo.Collection, filter bson.M, want int64) {
	t.Helper()
	got, err := collection.CountDocuments(t.Context(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d (filter=%v)", collection.Name(), got, want, filter)
	}
}
