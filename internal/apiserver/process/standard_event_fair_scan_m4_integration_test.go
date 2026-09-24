//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package process

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	qsmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	goNSQ "github.com/nsqio/go-nsq"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// The process proof calls this after its original container has stopped. It
// uses the same disposable databases, NSQD and production standard-profile
// constructor, but an independent subscriber channel for order observation.
func proveM4RelayHotFairness(t *testing.T, parent context.Context, gormDB *gorm.DB,
	mongoClient *mongo.Client, mongoDB *mongo.Database, catalog *eventcatalog.Catalog, cfg *config.Config,
	proxy *m4NSQFaultProxy,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	// Earlier subcases no longer use their rows. Isolate the runner's candidate
	// set so the observed positions refer only to this fixed mixed-state input.
	if _, err := sqlDB.ExecContext(ctx, "DELETE FROM rm_outbox"); err != nil {
		t.Fatal(err)
	}
	if _, err := mongoDB.Collection("rm_outbox").DeleteMany(ctx, bson.M{}); err != nil {
		t.Fatal(err)
	}
	const (
		initialHotPerStore = 64
		liveHotPerStore    = 96
		publishWorkers     = 2
		fairPositionBound  = 8
		fairTimeBound      = 5 * time.Second
	)
	proxy.SetResponseDelay(25 * time.Millisecond)
	defer proxy.SetResponseDelay(0)
	profile := eventsubsystem.ProfileOptions{Interval: 100 * time.Millisecond, PublishWorkers: publishWorkers}
	selected := *cfg.Eventing.StandardOutbox
	runtime, err := buildM4StandardEventSubsystem(eventsubsystem.Options{
		MySQLDB: gormDB, MongoDB: mongoDB, Catalog: catalog,
		MQPublisher: &fakePublisher{}, PublisherMode: eventruntime.PublishModeMQ,
		Mongo: profile, Assessment: profile,
		Consumers: map[string]eventsubsystem.ConsumerOptions{"modelcatalog.hot_rank_projection": {Enabled: false}},
	}, cfg, selected)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = runtime.Close()
		}
	}()
	mysqlBinding := runtime.Profile(eventcatalog.OutboxProfileAssessmentMySQL)
	mongoBinding := runtime.Profile(eventcatalog.OutboxProfileMongoDomain)
	makeEvent := func(id, kind, aggregate string, orgID int) event.DomainEvent {
		return event.Event[map[string]any]{
			BaseEvent: event.BaseEvent{ID: id, EventTypeValue: kind, AggregateTypeValue: aggregate,
				AggregateIDValue: id, OccurredAtValue: time.Now()},
			Data: map[string]any{"org_id": orgID},
		}
	}
	mysqlIDs := []string{"m4-fair-mysql-retry", "m4-fair-mysql-lease", "m4-fair-mysql-late-retry"}
	mongoIDs := []string{"m4-fair-mongo-retry", "m4-fair-mongo-lease", "m4-fair-mongo-late-lease"}
	mysqlEvents := []event.DomainEvent{
		makeEvent(mysqlIDs[0], "evaluation.requested", "Assessment", 501),
		makeEvent(mysqlIDs[1], "evaluation.requested", "Assessment", 502),
	}
	mongoEvents := []event.DomainEvent{
		makeEvent(mongoIDs[0], "answersheet.submitted", "AnswerSheet", 501),
		makeEvent(mongoIDs[1], "answersheet.submitted", "AnswerSheet", 502),
	}
	for i := 0; i < initialHotPerStore+liveHotPerStore; i++ {
		mysqlID := fmt.Sprintf("m4-fair-mysql-hot-%03d", i)
		mongoID := fmt.Sprintf("m4-fair-mongo-hot-%03d", i)
		mysqlIDs = append(mysqlIDs, mysqlID)
		mongoIDs = append(mongoIDs, mongoID)
		if i < initialHotPerStore {
			mysqlEvents = append(mysqlEvents, makeEvent(mysqlID, "evaluation.requested", "Assessment", 502))
			mongoEvents = append(mongoEvents, makeEvent(mongoID, "answersheet.submitted", "AnswerSheet", 502))
		}
	}
	if err := qsmysql.NewUnitOfWork(gormDB).WithinTransaction(ctx, func(txCtx context.Context) error {
		return mysqlBinding.Stager.Stage(txCtx, mysqlEvents...)
	}); err != nil {
		t.Fatal(err)
	}
	session, err := mongoClient.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	if _, err := session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (any, error) {
		return nil, mongoBinding.Stager.Stage(txCtx, mongoEvents...)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.ExecContext(ctx, `UPDATE rm_outbox SET state='retry_wait',
		next_attempt_at=TIMESTAMPADD(SECOND,-20,UTC_TIMESTAMP(6)),last_error_code='test_retry'
		WHERE message_id=?`, mysqlIDs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.ExecContext(ctx, `UPDATE rm_outbox SET state='publishing',
		lease_until=TIMESTAMPADD(SECOND,-10,UTC_TIMESTAMP(6)),
		next_attempt_at=TIMESTAMPADD(SECOND,-10,UTC_TIMESTAMP(6)),
		claim_token='stale-test-token',version=version+1,attempt_count=attempt_count+1
		WHERE message_id=?`, mysqlIDs[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := mongoDB.Collection("rm_outbox").UpdateOne(ctx, bson.M{"message_id": mongoIDs[0]}, bson.M{"$set": bson.M{
		"state": "retry_wait", "next_attempt_at": time.Now().Add(-20 * time.Second), "last_error_code": "test_retry",
	}}); err != nil {
		t.Fatal(err)
	}
	leaseDue := time.Now().Add(-10 * time.Second)
	if _, err := mongoDB.Collection("rm_outbox").UpdateOne(ctx, bson.M{"message_id": mongoIDs[1]}, bson.M{
		"$set": bson.M{"state": "publishing", "lease_until": leaseDue, "next_attempt_at": leaseDue, "claim_token": "stale-test-token"},
		"$inc": bson.M{"version": 1, "attempt_count": 1},
	}); err != nil {
		t.Fatal(err)
	}
	known := append(append([]string{}, mysqlIDs...), mongoIDs...)
	type delivery struct {
		id string
		at time.Time
	}
	seenCh := make(chan delivery, len(known))
	consumer, err := goNSQ.NewConsumer("qs.evaluation.lifecycle", fmt.Sprintf("m4-fair-%d", time.Now().UnixNano()), goNSQ.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Stop()
	consumer.AddHandler(goNSQ.HandlerFunc(func(message *goNSQ.Message) error {
		for _, id := range known {
			if bytes.Contains(message.Body, []byte(id)) {
				seenCh <- delivery{id: id, at: time.Now()}
				break
			}
		}
		return nil
	}))
	if err := consumer.ConnectToNSQD("nsqd:4150"); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := runtime.Start(ctx); err != nil {
		t.Fatal(err)
	}
	type injection struct {
		mysqlAt, mongoAt           time.Time
		mysqlPending, mongoPending int64
	}
	injectedCh := make(chan injection, 1)
	producerResult := make(chan error, 1)
	go func() {
		for i := initialHotPerStore; i < initialHotPerStore+liveHotPerStore; i++ {
			mysqlEvent := makeEvent(mysqlIDs[3+i], "evaluation.requested", "Assessment", 502)
			if err := qsmysql.NewUnitOfWork(gormDB).WithinTransaction(ctx, func(txCtx context.Context) error {
				return mysqlBinding.Stager.Stage(txCtx, mysqlEvent)
			}); err != nil {
				producerResult <- err
				return
			}
			mongoEvent := makeEvent(mongoIDs[3+i], "answersheet.submitted", "AnswerSheet", 502)
			if _, err := session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (any, error) {
				return nil, mongoBinding.Stager.Stage(txCtx, mongoEvent)
			}); err != nil {
				producerResult <- err
				return
			}
			if i == initialHotPerStore+8 {
				mysqlLate := makeEvent(mysqlIDs[2], "evaluation.requested", "Assessment", 501)
				if err := qsmysql.NewUnitOfWork(gormDB).WithinTransaction(ctx, func(txCtx context.Context) error {
					return mysqlBinding.Stager.Stage(txCtx, mysqlLate)
				}); err != nil {
					producerResult <- err
					return
				}
				if _, err := sqlDB.ExecContext(ctx, `UPDATE rm_outbox SET state='retry_wait',
					next_attempt_at=TIMESTAMPADD(SECOND,-30,UTC_TIMESTAMP(6)),last_error_code='test_retry'
					WHERE message_id=?`, mysqlIDs[2]); err != nil {
					producerResult <- err
					return
				}
				mysqlAt := time.Now()
				mongoLate := makeEvent(mongoIDs[2], "answersheet.submitted", "AnswerSheet", 501)
				if _, err := session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (any, error) {
					return nil, mongoBinding.Stager.Stage(txCtx, mongoLate)
				}); err != nil {
					producerResult <- err
					return
				}
				lateDue := time.Now().Add(-30 * time.Second)
				if _, err := mongoDB.Collection("rm_outbox").UpdateOne(ctx, bson.M{"message_id": mongoIDs[2]}, bson.M{
					"$set": bson.M{"state": "publishing", "lease_until": lateDue, "next_attempt_at": lateDue, "claim_token": "late-stale-token"},
					"$inc": bson.M{"version": 1, "attempt_count": 1},
				}); err != nil {
					producerResult <- err
					return
				}
				mongoAt := time.Now()
				var mysqlPending int64
				if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM rm_outbox
					WHERE message_id LIKE 'm4-fair-mysql-%' AND state='pending'`).Scan(&mysqlPending); err != nil {
					producerResult <- err
					return
				}
				mongoPending, err := mongoDB.Collection("rm_outbox").CountDocuments(ctx, bson.M{
					"message_id": bson.M{"$regex": "^m4-fair-mongo-"}, "state": "pending",
				})
				if err != nil {
					producerResult <- err
					return
				}
				if mysqlPending < 20 || mongoPending < 20 {
					producerResult <- fmt.Errorf("injected older intents without live backlog: mysql=%d mongo=%d", mysqlPending, mongoPending)
					return
				}
				injectedCh <- injection{mysqlAt: mysqlAt, mongoAt: mongoAt, mysqlPending: mysqlPending, mongoPending: mongoPending}
			}
			time.Sleep(20 * time.Millisecond)
		}
		producerResult <- nil
	}()
	seen := make(map[string]int, len(known))
	mysqlOrder := make([]delivery, 0, len(mysqlIDs))
	mongoOrder := make([]delivery, 0, len(mongoIDs))
	firstSeen := make(map[string]time.Duration, 4)
	seenAt := make(map[string]time.Time, 6)
	var injected injection
	injectedOK := false
	maxMySQLPublishing, maxMongoPublishing := int64(0), int64(0)
	latePositions := make(map[string]int, 2)
	sampleTicker := time.NewTicker(25 * time.Millisecond)
	defer sampleTicker.Stop()
	producerDone := false
	for len(seen) < len(known) || !producerDone || !injectedOK {
		select {
		case delivered := <-seenCh:
			seen[delivered.id]++
			if seen[delivered.id] > 1 {
				t.Fatalf("unexpected duplicate fair-scan delivery for %s", delivered.id)
			}
			if bytes.Contains([]byte(delivered.id), []byte("-mysql-")) {
				mysqlOrder = append(mysqlOrder, delivered)
			} else {
				mongoOrder = append(mongoOrder, delivered)
			}
			if delivered.id == mysqlIDs[0] || delivered.id == mysqlIDs[1] || delivered.id == mongoIDs[0] || delivered.id == mongoIDs[1] {
				firstSeen[delivered.id] = delivered.at.Sub(started)
			}
			if delivered.id == mysqlIDs[2] || delivered.id == mongoIDs[2] {
				seenAt[delivered.id] = delivered.at
			}
		case injected = <-injectedCh:
			injectedOK = true
			injectedCh = nil
		case err := <-producerResult:
			if err != nil {
				t.Fatalf("hot writer failed: %v", err)
			}
			producerDone = true
			producerResult = nil
		case <-sampleTicker.C:
			var mysqlPublishing int64
			if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM rm_outbox
				WHERE message_id LIKE 'm4-fair-mysql-%' AND state='publishing' AND lease_until > UTC_TIMESTAMP(6)`).Scan(&mysqlPublishing); err != nil {
				t.Fatal(err)
			}
			mongoPublishing, err := mongoDB.Collection("rm_outbox").CountDocuments(ctx, bson.M{
				"message_id": bson.M{"$regex": "^m4-fair-mongo-"}, "state": "publishing",
				"lease_until": bson.M{"$gt": time.Now()},
			})
			if err != nil {
				t.Fatal(err)
			}
			if mysqlPublishing > maxMySQLPublishing {
				maxMySQLPublishing = mysqlPublishing
			}
			if mongoPublishing > maxMongoPublishing {
				maxMongoPublishing = mongoPublishing
			}
			if mysqlPublishing > publishWorkers || mongoPublishing > publishWorkers {
				t.Fatalf("standard profile exceeded publish worker bounds: mysql=%d mongo=%d workers=%d",
					mysqlPublishing, mongoPublishing, publishWorkers)
			}
		case <-ctx.Done():
			t.Fatalf("hot-flow scan did not drain: seen=%d/%d mysql=%d mongo=%d: %v",
				len(seen), len(known), len(mysqlOrder), len(mongoOrder), ctx.Err())
		}
	}
	for _, tc := range []struct {
		name  string
		ids   []string
		order []delivery
	}{
		{"mysql", mysqlIDs[:2], mysqlOrder},
		{"mongo", mongoIDs[:2], mongoOrder},
	} {
		for _, id := range tc.ids {
			position := -1
			for i, delivered := range tc.order {
				if delivered.id == id {
					position = i + 1
					break
				}
			}
			if position < 1 || position > fairPositionBound || firstSeen[id] > fairTimeBound {
				t.Fatalf("%s older intent %s was not fairly delivered: position=%d/%d elapsed=%s/%s",
					tc.name, id, position, fairPositionBound, firstSeen[id], fairTimeBound)
			}
		}
	}
	for _, tc := range []struct {
		name  string
		id    string
		at    time.Time
		order []delivery
	}{
		{"mysql", mysqlIDs[2], injected.mysqlAt, mysqlOrder},
		{"mongo", mongoIDs[2], injected.mongoAt, mongoOrder},
	} {
		position := 0
		for _, delivered := range tc.order {
			if delivered.at.Before(tc.at) {
				continue
			}
			position++
			if delivered.id == tc.id {
				break
			}
		}
		elapsed := seenAt[tc.id].Sub(tc.at)
		latePositions[tc.id] = position
		if position < 1 || position > fairPositionBound || elapsed < 0 || elapsed > fairTimeBound {
			t.Fatalf("%s dynamically due intent %s starved behind hot flow: position=%d/%d elapsed=%s/%s pending_at_injection=%d/%d",
				tc.name, tc.id, position, fairPositionBound, elapsed, fairTimeBound,
				injected.mysqlPending, injected.mongoPending)
		}
	}
	if maxMySQLPublishing < 1 || maxMongoPublishing < 1 {
		t.Fatalf("publishing sampler did not observe active claims: mysql=%d mongo=%d", maxMySQLPublishing, maxMongoPublishing)
	}
	var mysqlPublished int
	var mongoPublished int64
	for {
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM rm_outbox WHERE message_id LIKE 'm4-fair-mysql-%' AND state='published'`).Scan(&mysqlPublished); err != nil {
			t.Fatal(err)
		}
		mongoPublished, err = mongoDB.Collection("rm_outbox").CountDocuments(ctx, bson.M{
			"message_id": bson.M{"$regex": "^m4-fair-mongo-"}, "state": "published",
		})
		if err != nil {
			t.Fatal(err)
		}
		if mysqlPublished == len(mysqlIDs) && mongoPublished == int64(len(mongoIDs)) {
			break
		}
		select {
		case <-time.After(25 * time.Millisecond):
		case <-ctx.Done():
			t.Fatalf("fair-scan broker delivery lacked state confirmation: mysql=%d/%d mongo=%d/%d: %v",
				mysqlPublished, len(mysqlIDs), mongoPublished, len(mongoIDs), ctx.Err())
		}
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	t.Logf("m4_05_hot_fairness mysql=%d mongo=%d live_per_store=%d pending_at_injection=mysql:%d,mongo:%d older_elapsed=%v late_position=%v late_elapsed=mysql:%s,mongo:%s max_publishing=mysql:%d,mongo:%d total=%s",
		len(mysqlIDs), len(mongoIDs), liveHotPerStore, injected.mysqlPending, injected.mongoPending,
		firstSeen, latePositions, seenAt[mysqlIDs[2]].Sub(injected.mysqlAt), seenAt[mongoIDs[2]].Sub(injected.mongoAt),
		maxMySQLPublishing, maxMongoPublishing, time.Since(started))
}
