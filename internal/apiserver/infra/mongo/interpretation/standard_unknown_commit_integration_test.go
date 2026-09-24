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
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	mongoevent "go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// The server commits the report transaction but returns one labeled unknown
// reply. WithTransaction must retry commit only, not the business callback.
func TestInterpretationStandardReportDriverUnknownCommitRetriesCommitOnly(t *testing.T) {
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
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if err := admin.RunCommand(cleanupCtx, bson.D{{Key: "configureFailPoint", Value: "failCommand"}, {Key: "mode", Value: "off"}}).Err(); err != nil {
			t.Errorf("disable Mongo failpoint: %v", err)
		}
	})
	commit(2)
	if commits.Load() != 2 || unknownReplies.Load() != 1 || writes.Load() != baselineWrites {
		t.Fatalf("unknown commit replayed business writes: commits=%d unknown=%d writes=%d baseline=%d",
			commits.Load(), unknownReplies.Load(), writes.Load(), baselineWrites)
	}
	assertMongoDocumentCount(t, monitoredDB.Collection("domain_event_outbox"), bson.M{}, 0)
}
