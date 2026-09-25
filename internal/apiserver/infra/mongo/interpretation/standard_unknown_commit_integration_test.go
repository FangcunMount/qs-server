//go:build integration && reliable_messaging_m4

package interpretation_test

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

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
	mongoevent "go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// The server commits a report success or automatic failure transaction but
// returns one labeled unknown reply. WithTransaction must retry commit only.
func TestInterpretationStandardDriverUnknownCommitRetriesCommitOnly(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	var commits, unknownReplies, writes atomic.Int32
	monitor := &mongoevent.CommandMonitor{
		Started: func(_ context.Context, evt *mongoevent.CommandStartedEvent) {
			switch evt.CommandName {
			case "commitTransaction":
				commits.Add(1)
			case "insert", "update", "findAndModify", "delete":
				writes.Add(1)
			}
		},
		Succeeded: func(_ context.Context, evt *mongoevent.CommandSucceededEvent) {
			if evt.CommandName == "commitTransaction" && evt.Reply.Lookup("writeConcernError").Type != 0 {
				unknownReplies.Add(1)
			}
		},
	}
	monitoredClient, err := mongo.Connect(ctx, options.Client().ApplyURI(os.Getenv("QS_SERVER_TEST_MONGO_URI")).SetMonitor(monitor))
	if err != nil {
		t.Fatal(err)
	}
	defer monitoredClient.Disconnect(context.Background())
	monitoredDB := monitoredClient.Database(db.Name())
	fixture := newInterpretationMongoFixture(t, monitoredDB)
	if err := monitoredDB.CreateCollection(ctx, "rm_outbox"); err != nil {
		t.Fatal(err)
	}
	wire, err := eventcatalog.Load("../../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stager, err := mongostandard.NewStager(monitoredDB.Collection("rm_outbox"), eventcatalog.NewCatalog(wire), eventruntime.SourceAPIServer)
	if err != nil {
		t.Fatal(err)
	}
	committer, err := execution.NewInterpretationCommitter(
		fixture.runner, fixture.generations, fixture.runs, fixture.reports, stager, nil, fixture.catalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	generation, run := fixture.start(t)
	commit := func(expectedIntents int64) {
		t.Helper()
		at := time.Now().Truncate(time.Millisecond)
		artifact := integrationArtifact(t, generation, run, at)
		commits.Store(0)
		unknownReplies.Store(0)
		writes.Store(0)
		result, err := committer.CommitSuccess(ctx, execution.CommitSuccessRequest{
			Generation: generation, Run: run, InterpretReport: artifact,
			BuilderIdentity:      domainreport.BuilderIdentityFactorScoring,
			ContentSchemaVersion: domainreport.ContentSchemaVersionV1, CompletedAt: at,
		})
		if err != nil || result == nil || result.Generation.Status() != domaingeneration.StatusGenerated ||
			result.Run.Status() != interpretationrun.StatusSucceeded {
			t.Fatalf("report commit result=%+v err=%v", result, err)
		}
		persisted, err := fixture.reports.FindByID(ctx, artifact.ID())
		if err != nil || persisted == nil {
			t.Fatalf("report was not committed: report=%+v err=%v", persisted, err)
		}
		assertMongoDocumentCount(t, monitoredDB.Collection("rm_outbox"),
			bson.M{"event_type": eventcatalog.InterpretationReportGenerated}, expectedIntents)
	}
	commit(1)
	baselineWrites := writes.Load()
	if commits.Load() != 1 || unknownReplies.Load() != 0 || baselineWrites == 0 {
		t.Fatalf("baseline commit commands=%d unknown=%d writes=%d", commits.Load(), unknownReplies.Load(), baselineWrites)
	}
	generation, run = fixture.start(t)
	admin := client.Database("admin")
	injectUnknownCommit := func() {
		t.Helper()
		if err := admin.RunCommand(ctx, bson.D{
			{Key: "configureFailPoint", Value: "failCommand"},
			{Key: "mode", Value: bson.M{"times": 1}},
			{Key: "data", Value: bson.M{
				"failCommands":      bson.A{"commitTransaction"},
				"writeConcernError": bson.M{"code": 64, "errmsg": "isolated unknown report commit result"},
				"errorLabels":       bson.A{"UnknownTransactionCommitResult"},
			}},
		}).Err(); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if err := admin.RunCommand(cleanupCtx, bson.D{{Key: "configureFailPoint", Value: "failCommand"}, {Key: "mode", Value: "off"}}).Err(); err != nil {
			t.Errorf("disable Mongo failpoint: %v", err)
		}
	})
	injectUnknownCommit()
	commit(2)
	if commits.Load() != 2 || unknownReplies.Load() != 1 || writes.Load() != baselineWrites {
		t.Fatalf("unknown commit replayed business writes: commits=%d unknown=%d writes=%d baseline=%d",
			commits.Load(), unknownReplies.Load(), writes.Load(), baselineWrites)
	}
	generation, run = fixture.start(t)
	commitFailure := func(expectedIntents int64) {
		t.Helper()
		failedAt := time.Now().Truncate(time.Millisecond)
		commits.Store(0)
		unknownReplies.Store(0)
		writes.Store(0)
		result, err := committer.CommitFailure(ctx, execution.CommitFailureRequest{
			Generation: generation, Run: run, OutcomeID: generation.Key().OutcomeID,
			Association: domainreport.Association{OrgID: 1, AssessmentID: meta.New(), TesteeID: 8},
			Failure: interpretationrun.Failure{
				Kind: interpretationrun.FailureKindBuild, Code: "build_failed", SafeMessage: "failed", Retryable: true,
			},
			FailedAt: failedAt,
		})
		if err != nil || result == nil || result.Generation.Status() != domaingeneration.StatusFailed ||
			result.Run.Status() != interpretationrun.StatusFailed {
			t.Fatalf("failure commit result=%+v err=%v", result, err)
		}
		decision := result.Run.RetryDecision()
		if decision == nil || decision.Disposition != retrygovernance.DispositionAutomatic || decision.NextAttemptAt == nil ||
			!decision.NextAttemptAt.After(failedAt) {
			t.Fatalf("automatic retry decision=%+v", decision)
		}
		persistedGeneration, err := fixture.generations.FindByID(ctx, generation.ID())
		if err != nil || persistedGeneration == nil || persistedGeneration.Status() != domaingeneration.StatusFailed {
			t.Fatalf("persisted failed generation=%+v err=%v", persistedGeneration, err)
		}
		persistedRun, err := fixture.runs.FindByID(ctx, run.ID())
		if err != nil || persistedRun == nil || persistedRun.Status() != interpretationrun.StatusFailed || persistedRun.RetryDecision() == nil ||
			persistedRun.RetryDecision().RetryEventID != decision.RetryEventID {
			t.Fatalf("persisted failed run=%+v err=%v", persistedRun, err)
		}
		collection := monitoredDB.Collection("rm_outbox")
		assertMongoDocumentCount(t, collection, bson.M{"event_type": eventcatalog.InterpretationReportFailed}, expectedIntents)
		assertMongoDocumentCount(t, collection, bson.M{"event_type": eventcatalog.InterpretationRetryRequested}, expectedIntents)
		var row struct {
			NextAttemptAt time.Time `bson:"next_attempt_at"`
		}
		if err := collection.FindOne(ctx, bson.M{"message_id": decision.RetryEventID}).Decode(&row); err != nil ||
			!row.NextAttemptAt.Equal(decision.NextAttemptAt.UTC().Truncate(time.Millisecond)) {
			t.Fatalf("scheduled retry due=%s decision=%+v err=%v", row.NextAttemptAt, decision, err)
		}
	}
	commitFailure(1)
	failureBaselineWrites := writes.Load()
	if commits.Load() != 1 || unknownReplies.Load() != 0 || failureBaselineWrites == 0 {
		t.Fatalf("failure baseline commits=%d unknown=%d writes=%d", commits.Load(), unknownReplies.Load(), failureBaselineWrites)
	}
	generation, run = fixture.start(t)
	injectUnknownCommit()
	commitFailure(2)
	if commits.Load() != 2 || unknownReplies.Load() != 1 || writes.Load() != failureBaselineWrites {
		t.Fatalf("unknown failure commit replayed writes: commits=%d unknown=%d writes=%d baseline=%d",
			commits.Load(), unknownReplies.Load(), writes.Load(), failureBaselineWrites)
	}
	assertMongoDocumentCount(t, monitoredDB.Collection("domain_event_outbox"), bson.M{}, 0)
}
