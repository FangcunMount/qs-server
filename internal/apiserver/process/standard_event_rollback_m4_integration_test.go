//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package process

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	legacyoutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	goNSQ "github.com/nsqio/go-nsq"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// This runs after the main process proof has closed its standard subsystem,
// against the same invocation-owned MySQL, Mongo replica set and NSQD. It
// tests the message ownership handoff; it does not assert a production cutover.
func proveM4RollbackHandoff(t *testing.T, db *gorm.DB, mongoClient *mongo.Client, mongoDB *mongo.Database,
	catalog *eventcatalog.Catalog, cfg *config.Config,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&legacyoutbox.OutboxPO{}); err != nil {
		t.Fatalf("prepare disposable legacy MySQL outbox: %v", err)
	}
	defer db.Migrator().DropTable(&legacyoutbox.OutboxPO{})

	const topic = "qs.evaluation.lifecycle"
	channel := fmt.Sprintf("m4-rollback-%d", time.Now().UnixNano())
	consumer, err := goNSQ.NewConsumer(topic, channel, goNSQ.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Stop()
	seenCh := make(chan string, 32)
	ids := []string{"m4-rollback-new-mysql", "m4-rollback-new-mongo", "m4-rollback-old-mysql", "m4-rollback-old-mongo"}
	consumer.AddHandler(goNSQ.HandlerFunc(func(message *goNSQ.Message) error {
		for _, id := range ids {
			if bytes.Contains(message.Body, []byte(id)) {
				seenCh <- id
				break
			}
		}
		return nil // go-nsq sends FIN after the handler returns.
	}))
	if err := consumer.ConnectToNSQD("nsqd:4150"); err != nil {
		t.Fatal(err)
	}

	baseOptions := func(publisher messaging.Publisher) eventsubsystem.Options {
		return eventsubsystem.Options{
			MySQLDB: db, MongoDB: mongoDB, Catalog: catalog,
			MQPublisher: publisher, PublisherMode: eventruntime.PublishModeMQ,
			Mongo: eventsubsystem.ProfileOptions{Interval: 100 * time.Millisecond, BatchSize: 4,
				PublishWorkers: 2, ImmediateMaxConcurrent: 2},
			Assessment: eventsubsystem.ProfileOptions{Interval: 100 * time.Millisecond, BatchSize: 4,
				PublishWorkers: 2, ImmediateMaxConcurrent: 2},
			Consumers: map[string]eventsubsystem.ConsumerOptions{"modelcatalog.hot_rank_projection": {Enabled: false}},
		}
	}
	newStandard := func() *eventsubsystem.Subsystem {
		t.Helper()
		selected := *cfg.Eventing.StandardOutbox
		candidate, err := buildM4StandardEventSubsystem(baseOptions(&fakePublisher{}), cfg, selected)
		if err != nil {
			t.Fatalf("start compatible standard version: %v", err)
		}
		return candidate
	}
	newLegacy := func() (*eventsubsystem.Subsystem, messaging.Publisher) {
		t.Helper()
		publisher, err := cfg.MessagingOptions.NewPublisher()
		if err != nil {
			t.Fatal(err)
		}
		legacy, err := eventsubsystem.New(baseOptions(publisher))
		if err != nil {
			_ = publisher.Close()
			t.Fatal(err)
		}
		return legacy, publisher
	}
	makeEvent := func(id, kind, aggregate, aggregateID string) event.Event[map[string]any] {
		return event.Event[map[string]any]{
			BaseEvent: event.BaseEvent{ID: id, EventTypeValue: kind, AggregateTypeValue: aggregate,
				AggregateIDValue: aggregateID, OccurredAtValue: time.Now()},
			Data: map[string]any{"org_id": 501},
		}
	}
	stage := func(runtime *eventsubsystem.Subsystem, mysqlEvent, mongoEvent event.Event[map[string]any]) {
		t.Helper()
		mySQL := runtime.Profile(eventcatalog.OutboxProfileAssessmentMySQL)
		if err := mysql.NewUnitOfWork(db).WithinTransaction(ctx, func(txCtx context.Context) error {
			return mySQL.Stager.Stage(txCtx, mysqlEvent)
		}); err != nil {
			t.Fatal(err)
		}
		mySQL.PostCommit.AfterCommit(ctx, []event.DomainEvent{mysqlEvent}, time.Now())
		mongoProfile := runtime.Profile(eventcatalog.OutboxProfileMongoDomain)
		session, err := mongoClient.StartSession()
		if err != nil {
			t.Fatal(err)
		}
		defer session.EndSession(ctx)
		if _, err := session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (any, error) {
			return nil, mongoProfile.Stager.Stage(txCtx, mongoEvent)
		}); err != nil {
			t.Fatal(err)
		}
		mongoProfile.PostCommit.AfterCommit(ctx, []event.DomainEvent{mongoEvent}, time.Now())
	}
	mysqlState := func(table, idColumn, id, stateColumn string) string {
		t.Helper()
		var state string
		query := fmt.Sprintf("SELECT %s FROM %s WHERE %s=?", stateColumn, table, idColumn)
		if err := sqlDB.QueryRowContext(ctx, query, id).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	mongoState := func(collection, idColumn, id, stateColumn string) string {
		t.Helper()
		var row bson.M
		if err := mongoDB.Collection(collection).FindOne(ctx, bson.M{idColumn: id}).Decode(&row); err != nil {
			t.Fatal(err)
		}
		state, _ := row[stateColumn].(string)
		return state
	}
	seen := map[string]int{}
	collect := func() {
		for {
			select {
			case id := <-seenCh:
				seen[id]++
			default:
				return
			}
		}
	}
	await := func(label string, required []string, check func() bool) {
		t.Helper()
		deadline := time.Now().Add(120 * time.Second)
		var channelStats m4RollbackChannelStats
		for {
			collect()
			if check() {
				allSeen := true
				for _, id := range required {
					if seen[id] == 0 {
						allSeen = false
					}
				}
				channelStats = readM4RollbackChannelStats(t, channel)
				if allSeen && consumer.Stats().MessagesFinished >= uint64(len(required)) &&
					channelStats.MessageCount >= uint64(len(required)) && channelStats.Depth == 0 &&
					channelStats.InFlight == 0 && channelStats.Deferred == 0 {
					return
				}
			}
			if time.Now().After(deadline) || ctx.Err() != nil {
				t.Fatalf("%s did not settle: seen=%v channel=%+v consumer=%+v context=%v",
					label, seen, channelStats, consumer.Stats(), ctx.Err())
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Admit two standard identities without starting their runner, then close
	// the candidate instance. Closing must not silently erase durable pending.
	writer := newStandard()
	stage(writer,
		makeEvent(ids[0], "evaluation.requested", "Assessment", "rollback-101"),
		makeEvent(ids[1], "answersheet.submitted", "AnswerSheet", "rollback-201"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if mysqlState("rm_outbox", "message_id", ids[0], "state") != "pending" ||
		mongoState("rm_outbox", "message_id", ids[1], "state") != "pending" {
		t.Fatal("closed standard instance lost or published unclaimed intent")
	}

	// An unsafe direct flip is observed only as a negative proof. The old
	// runners are then stopped before a compatible standard version restarts.
	oldProbe, oldPublisher := newLegacy()
	if err := oldProbe.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(750 * time.Millisecond)
	if mysqlState("rm_outbox", "message_id", ids[0], "state") != "pending" ||
		mongoState("rm_outbox", "message_id", ids[1], "state") != "pending" {
		t.Fatal("old profile unexpectedly changed a standard intent")
	}
	if err := oldProbe.Close(); err != nil {
		t.Fatal(err)
	}
	if err := oldPublisher.Close(); err != nil {
		t.Fatal(err)
	}

	recoveryStarted := time.Now()
	recovering := newStandard()
	if err := recovering.Start(ctx); err != nil {
		t.Fatal(err)
	}
	await("compatible standard recovery", ids[:2], func() bool {
		return mysqlState("rm_outbox", "message_id", ids[0], "state") == "published" &&
			mongoState("rm_outbox", "message_id", ids[1], "state") == "published"
	})
	if err := recovering.Close(); err != nil {
		t.Fatal(err)
	}
	standardRecovery := time.Since(recoveryStarted)

	// Only after the standard identities settle does the old profile resume.
	oldLive, livePublisher := newLegacy()
	if err := oldLive.Start(ctx); err != nil {
		t.Fatal(err)
	}
	stage(oldLive,
		makeEvent(ids[2], "evaluation.requested", "Assessment", "rollback-102"),
		makeEvent(ids[3], "answersheet.submitted", "AnswerSheet", "rollback-202"))
	await("legacy resume", ids, func() bool {
		return mysqlState("domain_event_outbox", "event_id", ids[2], "status") == "published" &&
			mongoState("domain_event_outbox", "event_id", ids[3], "status") == "published"
	})
	if err := oldLive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := livePublisher.Close(); err != nil {
		t.Fatal(err)
	}
	if standardRecovery > 120*time.Second {
		t.Fatalf("compatible standard recovery took %s, above 120s", standardRecovery)
	}
	t.Logf("m4_07_rollback standard_recovery=%s standard_intents=2 legacy_intents=2 client_fin=%d channel=%+v seen=%v",
		standardRecovery, consumer.Stats().MessagesFinished, readM4RollbackChannelStats(t, channel), seen)
}

type m4RollbackChannelStats struct {
	Name         string `json:"channel_name"`
	Depth        uint64 `json:"depth"`
	InFlight     uint64 `json:"in_flight_count"`
	Deferred     uint64 `json:"deferred_count"`
	MessageCount uint64 `json:"message_count"`
}

func readM4RollbackChannelStats(t *testing.T, channel string) m4RollbackChannelStats {
	t.Helper()
	response, err := (&http.Client{Timeout: 2 * time.Second}).Get("http://nsqd:4151/stats?format=json")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("NSQD stats returned %s", response.Status)
	}
	var stats struct {
		Topics []struct {
			Name     string                   `json:"topic_name"`
			Channels []m4RollbackChannelStats `json:"channels"`
		} `json:"topics"`
	}
	if err := json.NewDecoder(response.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	for _, topic := range stats.Topics {
		if topic.Name != "qs.evaluation.lifecycle" {
			continue
		}
		for _, found := range topic.Channels {
			if found.Name == channel {
				return found
			}
		}
	}
	t.Fatalf("isolated NSQ channel %s missing", channel)
	return m4RollbackChannelStats{}
}
