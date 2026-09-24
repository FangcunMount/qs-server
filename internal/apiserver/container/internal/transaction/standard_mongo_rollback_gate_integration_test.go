//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration

package transaction

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	"github.com/FangcunMount/reliable-messaging/transport"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/nsqio/go-nsq"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// This tests the release gate against four real SDK states. The first NSQ
// acceptance is deliberately recorded as unknown by the Outbox, so recovery
// must reuse the same event ID and duplicate delivery remains possible.
func TestM5MongoRollbackHoldsUntilStandardIntentsDrain(t *testing.T) {
	uri, nsqAddress := os.Getenv("RM_QS_ATTENTION_MONGO_URI"), os.Getenv("RM_QS_NSQ_TCP")
	if !strings.HasPrefix(uri, "mongodb://mongo:27017/") || !strings.Contains(uri, "replicaSet=rm-test") || nsqAddress != "nsqd:4150" {
		t.Fatal("disposable rm-test Mongo replica set and nsqd:4150 required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	db := client.Database("m5_qs_mongo_rollback")
	defer func() { _ = db.Drop(context.Background()) }()
	for _, name := range []string{"rm_outbox", "qs_rm_replay_requests"} {
		if err := db.CreateCollection(ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	collection := db.Collection("rm_outbox")
	if _, err := collection.Indexes().CreateMany(ctx, sdkmongo.Indexes()); err != nil {
		t.Fatal(err)
	}
	catalog, err := eventcatalog.Load("/configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stager, err := mongostandard.NewStager(collection, eventcatalog.NewCatalog(catalog), eventruntime.SourceAPIServer)
	if err != nil {
		t.Fatal(err)
	}
	store, err := sdkmongo.New(collection)
	if err != nil {
		t.Fatal(err)
	}
	const topic = "qs.evaluation.lifecycle"
	config := nsq.NewConfig()
	config.HeartbeatInterval, config.MsgTimeout = time.Second, 10*time.Second
	config.ReadTimeout, config.WriteTimeout = 3*time.Second, time.Second
	consumer, err := nsq.NewConsumer(topic, "rm-m5-rollback-gate", config)
	if err != nil {
		t.Fatal(err)
	}
	consumer.SetLogger(nil, nsq.LogLevelError)
	type delivery struct {
		id  string
		err error
	}
	delivered := make(chan delivery, 16)
	consumer.AddHandler(nsq.HandlerFunc(func(raw *nsq.Message) error {
		wire, recognized, decodeErr := messaging.DecodeMessagePayload(raw.Body)
		if decodeErr == nil && !recognized {
			decodeErr = fmt.Errorf("standard NSQ envelope not recognized")
		}
		id := ""
		if decodeErr == nil {
			id = wire.UUID
			if wire.Metadata["event_type"] != eventcatalog.AnswerSheetSubmitted {
				decodeErr = fmt.Errorf("unexpected event type %q", wire.Metadata["event_type"])
			}
		}
		select {
		case delivered <- delivery{id: id, err: decodeErr}:
		case <-ctx.Done():
			return ctx.Err()
		}
		return decodeErr
	}))
	if err := consumer.ConnectToNSQD(nsqAddress); err != nil {
		t.Fatal(err)
	}
	defer func() {
		consumer.Stop()
		select {
		case <-consumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("NSQ consumer did not stop")
		}
	}()
	producer, err := nsq.NewProducer(nsqAddress, config)
	if err != nil {
		t.Fatal(err)
	}
	producer.SetLogger(nil, nsq.LogLevelError)
	defer producer.Stop()
	publisher, err := sdknsq.New(producer, map[string]string{topic: topic}, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := publisher.Drain(ctx); err != nil {
			t.Errorf("publisher drain: %v", err)
		}
	}()
	stage := func(id string) {
		t.Helper()
		evt := event.Event[map[string]any]{
			BaseEvent: event.BaseEvent{ID: id, EventTypeValue: eventcatalog.AnswerSheetSubmitted,
				OccurredAtValue: time.Now(), AggregateTypeValue: "AnswerSheet", AggregateIDValue: id},
			Data: map[string]any{"org_id": 501},
		}
		session, err := client.StartSession()
		if err != nil {
			t.Fatal(err)
		}
		defer session.EndSession(ctx)
		_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
			return nil, stager.Stage(tx, evt)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	const unknownID = "m5-rollback-unknown"
	const publishingID = "m5-rollback-publishing"
	const quarantineID = "m5-rollback-quarantined"
	const pendingID = "m5-rollback-pending"
	stage(unknownID)
	claimed, err := store.ClaimDue(ctx, 1, 2*time.Second)
	if err != nil || len(claimed) != 1 || claimed[0].Message.Input().ID != unknownID {
		t.Fatalf("claim unknown: count=%d err=%v", len(claimed), err)
	}
	if result := publisher.Publish(ctx, claimed[0].Message); result.Outcome != transport.Confirmed {
		t.Fatalf("initial NSQ acceptance: %v", result.Outcome)
	}
	if err := store.Retry(ctx, claimed[0], 5*time.Second, "publish_unknown"); err != nil {
		t.Fatal(err)
	}
	stage(publishingID)
	claimed, err = store.ClaimDue(ctx, 1, 5*time.Second)
	if err != nil || len(claimed) != 1 || claimed[0].Message.Input().ID != publishingID {
		t.Fatalf("claim publishing: count=%d err=%v", len(claimed), err)
	}
	stage(quarantineID)
	quarantined, err := store.ClaimDue(ctx, 1, 5*time.Second)
	if err != nil || len(quarantined) != 1 || quarantined[0].Message.Input().ID != quarantineID {
		t.Fatalf("claim quarantine: count=%d err=%v", len(quarantined), err)
	}
	if err := store.Quarantine(ctx, quarantined[0], "publish_unknown"); err != nil {
		t.Fatal(err)
	}
	stage(pendingID)
	readUnfinished := func() map[string]string {
		t.Helper()
		cursor, err := collection.Find(ctx, bson.M{"state": bson.M{"$ne": "published"}},
			options.Find().SetProjection(bson.M{"producer": 1, "message_id": 1, "destination": 1,
				"event_type": 1, "scope": 1, "state": 1, "last_error_code": 1,
				"version": 1, "next_attempt_at": 1, "lease_until": 1, "_id": 0}))
		if err != nil {
			t.Fatal(err)
		}
		defer cursor.Close(ctx)
		var rows []struct {
			MessageID string `bson:"message_id"`
			State     string `bson:"state"`
			Code      string `bson:"last_error_code"`
		}
		if err := cursor.All(ctx, &rows); err != nil {
			t.Fatal(err)
		}
		states := make(map[string]string, len(rows))
		for _, row := range rows {
			states[row.MessageID] = row.State + "/" + row.Code
		}
		return states
	}
	initial := readUnfinished()
	if len(initial) != 4 || initial[unknownID] != "retry_wait/publish_unknown" ||
		initial[publishingID] != "publishing/" || initial[quarantineID] != "quarantined/publish_unknown" ||
		initial[pendingID] != "pending/" {
		t.Fatalf("unsafe rollback was not blocked by all standard states: %+v", initial)
	}
	startRelay := func() (context.CancelFunc, <-chan error) {
		t.Helper()
		forwarder, err := relay.New(store, publisher, relay.Config{
			Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: 4 * time.Second,
			PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
			Retry: standardoutbox.SDKRetryPolicy(), Observe: func(relay.Event) {},
		})
		if err != nil {
			t.Fatal(err)
		}
		runCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- forwarder.Run(runCtx) }()
		return stop, done
	}
	stop, done := startRelay()
	defer stop()
	seen := map[string]int{}
	await := func(want map[string]int, states map[string]string) {
		t.Helper()
		for {
			select {
			case got := <-delivered:
				if got.err != nil {
					t.Fatalf("NSQ delivery: %v", got.err)
				}
				seen[got.id]++
			case <-time.After(50 * time.Millisecond):
			}
			settled := true
			for id, count := range want {
				if seen[id] < count {
					settled = false
				}
			}
			for id, state := range states {
				var row struct {
					State string `bson:"state"`
				}
				if err := collection.FindOne(ctx, bson.M{"message_id": id}).Decode(&row); err != nil || row.State != state {
					settled = false
				}
			}
			if settled {
				return
			}
			if ctx.Err() != nil {
				t.Fatalf("compatible recovery did not settle: deliveries=%v states=%v err=%v", seen, readUnfinished(), ctx.Err())
			}
		}
	}
	await(map[string]int{unknownID: 2, publishingID: 1, pendingID: 1},
		map[string]string{unknownID: "published", publishingID: "published", pendingID: "published", quarantineID: "quarantined"})
	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	remaining := readUnfinished()
	if len(remaining) != 1 || remaining[quarantineID] != "quarantined/publish_unknown" || seen[quarantineID] != 0 {
		t.Fatalf("quarantined message was lost or automatically replayed: states=%v deliveries=%v", remaining, seen)
	}
	reader, err := mongostandard.NewStatusReader(collection)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := reader.ListOutboxCandidates(ctx, 501, 10)
	if err != nil || len(candidates) != 1 || candidates[0].ResourceID != quarantineID || candidates[0].Disposition != "manual_required" {
		t.Fatalf("quarantined message missing from governance candidates: items=%+v err=%v", candidates, err)
	}
	ledger, err := mongostandard.NewReplayLedger(db, "mongo-domain-events")
	if err != nil {
		t.Fatal(err)
	}
	approved, err := ledger.AuthorizeManualReplayWithReason(ctx, 501, "m5-rollback-review", "isolated rollback proof",
		[]outboxport.ManualReplayTarget{{EventID: quarantineID, ExpectedAttemptCount: 1}})
	if err != nil || len(approved) != 1 || !approved[0].Authorized {
		t.Fatalf("audited quarantine authorization: items=%+v err=%v", approved, err)
	}
	stop, done = startRelay()
	defer stop()
	await(map[string]int{unknownID: 2, publishingID: 1, pendingID: 1, quarantineID: 1},
		map[string]string{unknownID: "published", publishingID: "published", pendingID: "published", quarantineID: "published"})
	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if left := readUnfinished(); len(left) != 0 {
		t.Fatalf("standard intents still block old Profile: %+v", left)
	}
	if seen[unknownID] != 2 || seen[publishingID] != 1 || seen[pendingID] != 1 || seen[quarantineID] != 1 {
		t.Fatalf("unexpected broker deliveries: %+v", seen)
	}
	for consumer.Stats().MessagesFinished < 5 && ctx.Err() == nil {
		time.Sleep(10 * time.Millisecond)
	}
	if consumer.Stats().MessagesFinished != 5 {
		t.Fatalf("broker messages were not all FINed: %+v", consumer.Stats())
	}
	if err := db.Collection("qs_rm_replay_requests").FindOne(ctx, bson.M{"request_id": "m5-rollback-review"}).Err(); err != nil {
		t.Fatalf("manual replay audit missing: %v", err)
	}
	t.Logf("standard drain verified before old Profile: states=%v deliveries=%v", readUnfinished(), seen)
}
