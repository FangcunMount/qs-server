//go:build integration && reliable_messaging_m4

package interpretation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	execution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"go.mongodb.org/mongo-driver/bson"
)

func TestInterpretationAutomaticFailureCommitsWithStandardMongoScheduledRetry(t *testing.T) {
	fixture, stager := newStandardInterpretationFixture(t)
	db := fixture.db
	generation, run := fixture.start(t)
	fixture.now = time.Now().Add(time.Second).Truncate(time.Millisecond)
	committer, err := execution.NewInterpretationCommitter(
		fixture.runner, fixture.generations, fixture.runs, fixture.reports, stager, nil, fixture.catalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := committer.CommitFailure(t.Context(), execution.CommitFailureRequest{
		Generation: generation, Run: run, OutcomeID: generation.Key().OutcomeID,
		Association: domainreport.Association{OrgID: 1, AssessmentID: meta.New(), TesteeID: 8},
		Failure: interpretationrun.Failure{
			Kind: interpretationrun.FailureKindBuild, Code: "build_failed", SafeMessage: "failed", Retryable: true,
		},
		FailedAt: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation.Status() != domaingeneration.StatusFailed || result.Run.Status() != interpretationrun.StatusFailed {
		t.Fatalf("committed state generation=%s run=%s", result.Generation.Status(), result.Run.Status())
	}
	decision := result.Run.RetryDecision()
	if decision == nil || decision.Disposition != retrygovernance.DispositionAutomatic || decision.NextAttemptAt == nil || !decision.NextAttemptAt.After(fixture.now) {
		t.Fatalf("automatic retry decision=%+v", decision)
	}
	persistedGeneration, err := fixture.generations.FindByID(t.Context(), generation.ID())
	if err != nil {
		t.Fatal(err)
	}
	persistedRun, err := fixture.runs.FindByID(t.Context(), run.ID())
	if err != nil {
		t.Fatal(err)
	}
	if persistedGeneration.Status() != domaingeneration.StatusFailed || persistedRun.Status() != interpretationrun.StatusFailed {
		t.Fatalf("persisted state generation=%s run=%s", persistedGeneration.Status(), persistedRun.Status())
	}
	collection := db.Collection("rm_outbox")
	count, err := collection.CountDocuments(t.Context(), bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("standard Mongo intents=%d, want failure and scheduled retry", count)
	}
	for _, eventType := range []string{eventcatalog.InterpretationReportFailed, eventcatalog.InterpretationRetryRequested} {
		var row struct {
			MessageID     string    `bson:"message_id"`
			Scope         string    `bson:"scope"`
			State         string    `bson:"state"`
			NextAttemptAt time.Time `bson:"next_attempt_at"`
		}
		if err := collection.FindOne(t.Context(), bson.M{"event_type": eventType}).Decode(&row); err != nil {
			t.Fatal(err)
		}
		if row.Scope != "org:1" || row.State != "pending" {
			t.Fatalf("%s scope=%s state=%s", eventType, row.Scope, row.State)
		}
		if eventType == eventcatalog.InterpretationRetryRequested {
			if row.MessageID != decision.RetryEventID || !row.NextAttemptAt.Equal(decision.NextAttemptAt.UTC().Truncate(time.Millisecond)) {
				t.Fatalf("scheduled retry message=%s due=%s; decision=%+v", row.MessageID, row.NextAttemptAt, decision)
			}
		}
	}
	legacyCount, err := db.Collection("domain_event_outbox").CountDocuments(t.Context(), bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 {
		t.Fatalf("standard failure wrote %d legacy intents", legacyCount)
	}

	abortedGeneration, abortedRun := fixture.start(t)
	fixture.now = time.Now().Add(time.Second).Truncate(time.Millisecond)
	abort := errors.New("abort after scheduled stage")
	failing, err := execution.NewInterpretationCommitter(
		fixture.runner, fixture.generations, fixture.runs, fixture.reports,
		failAfterScheduledStage{inner: stager, failure: abort}, nil, fixture.catalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = failing.CommitFailure(t.Context(), execution.CommitFailureRequest{
		Generation: abortedGeneration, Run: abortedRun, OutcomeID: abortedGeneration.Key().OutcomeID,
		Association: domainreport.Association{OrgID: 1, AssessmentID: meta.New(), TesteeID: 8},
		Failure: interpretationrun.Failure{
			Kind: interpretationrun.FailureKindBuild, Code: "build_failed", SafeMessage: "failed", Retryable: true,
		},
		FailedAt: fixture.now,
	})
	if !errors.Is(err, abort) {
		t.Fatalf("scheduled-stage failure error=%v", err)
	}
	fixture.assertRunningAndNoOutbox(t, abortedGeneration, abortedRun)
	count, err = collection.CountDocuments(t.Context(), bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("aborted failure left %d standard intents; want prior 2", count)
	}
}

func TestInterpretationSuccessCommitsReportWithStandardMongoIntent(t *testing.T) {
	fixture, stager := newStandardInterpretationFixture(t)
	generation, run := fixture.start(t)
	at := time.Now().Add(time.Second).Truncate(time.Millisecond)
	artifact := integrationArtifact(t, generation, run, at)
	committer, err := execution.NewInterpretationCommitter(
		fixture.runner, fixture.generations, fixture.runs, fixture.reports, stager, nil, fixture.catalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := committer.CommitSuccess(t.Context(), execution.CommitSuccessRequest{
		Generation: generation, Run: run, InterpretReport: artifact,
		BuilderIdentity:      domainreport.BuilderIdentityFactorScoring,
		ContentSchemaVersion: domainreport.ContentSchemaVersionV1,
		CompletedAt:          at,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation.Status() != domaingeneration.StatusGenerated || result.Run.Status() != interpretationrun.StatusSucceeded {
		t.Fatalf("committed state generation=%s run=%s", result.Generation.Status(), result.Run.Status())
	}
	persistedReport, err := fixture.reports.FindByID(t.Context(), artifact.ID())
	if err != nil || persistedReport == nil {
		t.Fatalf("persisted report=%v err=%v", persistedReport, err)
	}
	assertMongoDocumentCount(t, fixture.db.Collection("rm_outbox"), bson.M{"event_type": eventcatalog.InterpretationReportGenerated}, 1)
	assertMongoDocumentCount(t, fixture.db.Collection("domain_event_outbox"), bson.M{}, 0)
}

func newStandardInterpretationFixture(t *testing.T) (interpretationMongoFixture, *mongostandard.Stager) {
	t.Helper()
	_, db := mongodbtest.ReplicaSetDatabase(t)
	fixture := newInterpretationMongoFixture(t, db)
	if err := db.CreateCollection(t.Context(), "rm_outbox"); err != nil {
		t.Fatal(err)
	}
	wire, err := eventcatalog.Load("../../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stager, err := mongostandard.NewStager(db.Collection("rm_outbox"), eventcatalog.NewCatalog(wire), eventruntime.SourceAPIServer)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, stager
}

type failAfterScheduledStage struct {
	inner   *mongostandard.Stager
	failure error
}

func (s failAfterScheduledStage) Stage(ctx context.Context, events ...event.DomainEvent) error {
	return s.inner.Stage(ctx, events...)
}

func (s failAfterScheduledStage) StageAt(ctx context.Context, dueAt time.Time, events ...event.DomainEvent) error {
	if err := s.inner.StageAt(ctx, dueAt, events...); err != nil {
		return err
	}
	return s.failure
}
