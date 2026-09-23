//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package standardoutbox

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	request "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/reliable-messaging/message"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	driver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestStandardMongoReplayLedgerCrashAndRollback(t *testing.T) {
	uri := os.Getenv("RM_QS_REPLAY_MONGO_URI")
	if !strings.HasPrefix(uri, "mongodb://mongo:27017/") || !strings.Contains(uri, "replicaSet=rm-test") {
		t.Fatal("disposable rm-test replica-set Mongo URI required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	client, err := driver.Connect(ctx, options.Client().ApplyURI(uri))
	must(err)
	defer client.Disconnect(context.Background())
	db := client.Database("m4_qs_mongo_replay")
	defer db.Drop(context.Background())
	outbox := db.Collection("rm_outbox")
	must(db.CreateCollection(ctx, "rm_outbox"))
	must(db.CreateCollection(ctx, "qs_rm_replay_requests"))
	_, err = outbox.Indexes().CreateMany(ctx, sdkmongo.Indexes())
	must(err)
	_, err = outbox.Indexes().CreateOne(ctx, driver.IndexModel{Keys: bson.D{{Key: "message_id", Value: 1}}})
	must(err)
	appendQuarantined := func(id, scope, code string, failures int64) {
		t.Helper()
		m, err := message.New(message.Input{
			Producer: "qs-server", ID: id, Destination: "qs.evaluation.lifecycle",
			EventType: "evaluation.requested", SchemaVersion: "v1", Scope: scope,
			ContentType: "application/json", OccurredAt: "2026-09-23T10:00:00+08:00",
			Payload: []byte(`{"event_id":"` + id + `"}`),
		})
		must(err)
		session, err := client.StartSession()
		must(err)
		defer session.EndSession(ctx)
		_, err = session.WithTransaction(ctx, func(sc driver.SessionContext) (interface{}, error) {
			a, err := sdkmongo.Bind(sc, outbox)
			if err != nil {
				return nil, err
			}
			return nil, a.Append(m, time.Now().Add(-time.Minute))
		})
		must(err)
		_, err = outbox.UpdateOne(ctx, bson.M{"message_id": id}, bson.M{"$set": bson.M{
			"state": "quarantined", "last_error_code": code, "failure_count": failures,
		}})
		must(err)
	}
	ledger, err := NewReplayLedger(db, "mongo-domain-events")
	must(err)
	appendQuarantined("event-a", "org:7", "publish_unknown", 30)
	first := request.ReplayRequest{
		OrgID: 7, RequestID: "request-1", Store: "mongo-domain-events", Reason: "reviewed by operator",
		Targets: []request.ReplayTarget{{EventID: "event-a", ExpectedFailureCount: 30}},
	}
	results, err := ledger.Authorize(ctx, first)
	must(err)
	if len(results) != 1 || !results[0].Authorized {
		t.Fatalf("first authorization: %+v", results)
	}
	read := func(id string, wantState, wantManualID string, wantFailures, wantVersion uint64) {
		t.Helper()
		var row struct {
			State                 string `bson:"state"`
			ManualReplayRequestID string `bson:"manual_replay_request_id"`
			FailureCount          uint64 `bson:"failure_count"`
			Version               uint64 `bson:"version"`
			AttemptCount          uint64 `bson:"attempt_count"`
			Payload               []byte `bson:"payload"`
		}
		must(outbox.FindOne(ctx, bson.M{"message_id": id}).Decode(&row))
		if row.State != wantState || row.ManualReplayRequestID != wantManualID ||
			row.FailureCount != wantFailures || row.Version != wantVersion ||
			row.AttemptCount != 0 || string(row.Payload) != `{"event_id":"`+id+`"}` {
			t.Fatalf("%s row changed unexpectedly: %+v", id, row)
		}
	}
	read("event-a", "retry_wait", first.RequestID, 30, 1)
	_, err = outbox.UpdateOne(ctx, bson.M{"message_id": "event-a"}, bson.M{"$set": bson.M{
		"state": "quarantined", "last_error_code": "publish_unknown", "failure_count": int64(31),
	}})
	must(err)
	second := first
	second.RequestID = "request-2"
	second.Targets = []request.ReplayTarget{{EventID: "event-a", ExpectedFailureCount: 31}}
	results, err = ledger.Authorize(ctx, second)
	must(err)
	if len(results) != 1 || !results[0].Authorized {
		t.Fatalf("second authorization: %+v", results)
	}
	replayed, found, err := ledger.Resolve(ctx, first)
	must(err)
	if !found || len(replayed) != 1 || !replayed[0].Authorized {
		t.Fatalf("lost first durable result: found=%t items=%+v", found, replayed)
	}
	replayed, err = ledger.Authorize(ctx, first)
	must(err)
	if len(replayed) != 1 || !replayed[0].Authorized {
		t.Fatalf("same request could not recover prior result: %+v", replayed)
	}
	read("event-a", "retry_wait", second.RequestID, 31, 2)
	changed := first
	changed.Reason = "different reason"
	if _, err := ledger.Authorize(ctx, changed); !errors.Is(err, request.ErrReplayInputConflict) {
		t.Fatalf("changed reason reused request ID: %v", err)
	}
	changed = first
	changed.Targets = []request.ReplayTarget{{EventID: "event-a", ExpectedFailureCount: 31}}
	if _, err := ledger.Authorize(ctx, changed); !errors.Is(err, request.ErrReplayInputConflict) {
		t.Fatalf("changed target reused request ID: %v", err)
	}
	appendQuarantined("other-org", "org:8", "publish_unknown", 30)
	appendQuarantined("terminal", "org:7", "publish_rejected", 1)
	appendQuarantined("corrupt", "org:7", "invalid_immutable_content", 1)
	denied := request.ReplayRequest{
		OrgID: 7, RequestID: "request-denied", Store: "mongo-domain-events", Reason: "reviewed",
		Targets: []request.ReplayTarget{
			{EventID: "other-org", ExpectedFailureCount: 30},
			{EventID: "terminal", ExpectedFailureCount: 1},
			{EventID: "corrupt", ExpectedFailureCount: 1},
			{EventID: "missing", ExpectedFailureCount: 30},
		},
	}
	results, err = ledger.Authorize(ctx, denied)
	must(err)
	if len(results) != 4 || results[0].Reason != "organization_mismatch" ||
		results[1].Reason != "not_manual_required" || results[2].Reason != "not_manual_required" ||
		results[3].Reason != "not_found" {
		t.Fatalf("denial categories changed: %+v", results)
	}
	appendQuarantined("rollback-a", "org:7", "publish_unknown", 30)
	appendQuarantined("rollback-b", "org:7", "publish_unknown", 30)
	// The validator rejects only the second transition. Mongo must roll back
	// the earlier message update and the request document in the same session.
	must(db.RunCommand(ctx, bson.D{
		{Key: "collMod", Value: "rm_outbox"},
		{Key: "validator", Value: bson.M{"$or": bson.A{
			bson.M{"message_id": bson.M{"$ne": "rollback-b"}},
			bson.M{"state": bson.M{"$ne": "retry_wait"}},
		}}},
		{Key: "validationAction", Value: "error"},
	}).Err())
	batch := request.ReplayRequest{
		OrgID: 7, RequestID: "request-rollback", Store: "mongo-domain-events", Reason: "reviewed",
		Targets: []request.ReplayTarget{
			{EventID: "rollback-a", ExpectedFailureCount: 30},
			{EventID: "rollback-b", ExpectedFailureCount: 30},
		},
	}
	if _, err := ledger.Authorize(ctx, batch); err == nil {
		t.Fatal("controlled second-target failure unexpectedly committed")
	}
	read("rollback-a", "quarantined", "", 30, 0)
	read("rollback-b", "quarantined", "", 30, 0)
	_, found, err = ledger.Resolve(ctx, batch)
	must(err)
	if found {
		t.Fatal("failed batch left committed request header")
	}
	must(db.RunCommand(ctx, bson.D{{Key: "collMod", Value: "rm_outbox"}, {Key: "validator", Value: bson.M{}}}).Err())
	results, err = ledger.Authorize(ctx, batch)
	must(err)
	if len(results) != 2 || !results[0].Authorized || !results[1].Authorized {
		t.Fatalf("retry after rollback: %+v", results)
	}
	appendQuarantined("concurrent", "org:7", "publish_unknown", 30)
	parallel := request.ReplayRequest{
		OrgID: 7, RequestID: "request-concurrent", Store: "mongo-domain-events", Reason: "reviewed",
		Targets: []request.ReplayTarget{{EventID: "concurrent", ExpectedFailureCount: 30}},
	}
	type outcome struct {
		items []request.ReplayResult
		err   error
	}
	outcomes := make(chan outcome, 2)
	for range 2 {
		go func() {
			items, err := ledger.Authorize(ctx, parallel)
			outcomes <- outcome{items, err}
		}()
	}
	for range 2 {
		got := <-outcomes
		must(got.err)
		if len(got.items) != 1 || !got.items[0].Authorized {
			t.Fatalf("parallel request lost original result: %+v", got.items)
		}
	}
	read("concurrent", "retry_wait", parallel.RequestID, 30, 1)
	appendQuarantined("callback-retry", "org:7", "publish_unknown", 30)
	admin := client.Database("admin")
	var beforeFailpoint struct {
		Count int64 `bson:"count"`
	}
	must(admin.RunCommand(ctx, bson.D{
		{Key: "configureFailPoint", Value: "failCommand"},
		{Key: "mode", Value: bson.M{"times": 1}},
		{Key: "data", Value: bson.M{
			"failCommands": bson.A{"update"}, "errorCode": 112,
			"errorLabels": bson.A{"TransientTransactionError"},
		}},
	}).Decode(&beforeFailpoint))
	defer admin.RunCommand(context.Background(), bson.D{
		{Key: "configureFailPoint", Value: "failCommand"}, {Key: "mode", Value: "off"},
	})
	retried := request.ReplayRequest{
		OrgID: 7, RequestID: "request-callback-retry", Store: "mongo-domain-events", Reason: "reviewed",
		Targets: []request.ReplayTarget{{EventID: "callback-retry", ExpectedFailureCount: 30}},
	}
	results, err = ledger.Authorize(ctx, retried)
	must(err)
	if len(results) != 1 || !results[0].Authorized {
		t.Fatalf("transient callback retry: %+v", results)
	}
	var afterFailpoint struct {
		Count int64 `bson:"count"`
	}
	must(admin.RunCommand(ctx, bson.D{
		{Key: "configureFailPoint", Value: "failCommand"}, {Key: "mode", Value: "off"},
	}).Decode(&afterFailpoint))
	if afterFailpoint.Count <= beforeFailpoint.Count {
		t.Fatalf("transient error was not injected: before=%d after=%d", beforeFailpoint.Count, afterFailpoint.Count)
	}
	read("callback-retry", "retry_wait", retried.RequestID, 30, 1)
	appendQuarantined("commit-unknown", "org:7", "publish_unknown", 30)
	var commits, unknownReplies atomic.Int32
	monitor := &event.CommandMonitor{
		Started: func(_ context.Context, evt *event.CommandStartedEvent) {
			if evt.CommandName == "commitTransaction" {
				commits.Add(1)
			}
		},
		Succeeded: func(_ context.Context, evt *event.CommandSucceededEvent) {
			if evt.CommandName == "commitTransaction" && evt.Reply.Lookup("writeConcernError").Type != 0 {
				unknownReplies.Add(1)
			}
		},
	}
	monitoredClient, err := driver.Connect(ctx, options.Client().ApplyURI(uri).SetMonitor(monitor))
	must(err)
	defer monitoredClient.Disconnect(context.Background())
	monitoredLedger, err := NewReplayLedger(monitoredClient.Database("m4_qs_mongo_replay"), "mongo-domain-events")
	must(err)
	must(admin.RunCommand(ctx, bson.D{
		{Key: "configureFailPoint", Value: "failCommand"},
		{Key: "mode", Value: bson.M{"times": 1}},
		{Key: "data", Value: bson.M{
			"failCommands":      bson.A{"commitTransaction"},
			"writeConcernError": bson.M{"code": 64, "errmsg": "isolated unknown commit result"},
			"errorLabels":       bson.A{"UnknownTransactionCommitResult"},
		}},
	}).Err())
	unknown := request.ReplayRequest{
		OrgID: 7, RequestID: "request-commit-unknown", Store: "mongo-domain-events", Reason: "reviewed",
		Targets: []request.ReplayTarget{{EventID: "commit-unknown", ExpectedFailureCount: 30}},
	}
	results, err = monitoredLedger.Authorize(ctx, unknown)
	must(err)
	if len(results) != 1 || !results[0].Authorized || commits.Load() != 2 || unknownReplies.Load() != 1 {
		t.Fatalf("unknown commit recovery: results=%+v commits=%d unknown=%d", results, commits.Load(), unknownReplies.Load())
	}
	read("commit-unknown", "retry_wait", unknown.RequestID, 30, 1)
	recovered, found, err := ledger.Resolve(ctx, unknown)
	must(err)
	if !found || len(recovered) != 1 || !recovered[0].Authorized {
		t.Fatalf("unknown commit left no durable result: found=%t items=%+v", found, recovered)
	}
}
