//go:build integration

package transport

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	cbnsq "github.com/FangcunMount/component-base/pkg/messaging/nsq"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	workermessaging "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/nsqio/go-nsq"
)

type workerSettlementRuntime struct {
	topic        string
	unknownCalls atomic.Int32
	failedCalls  atomic.Int32
}

func (r *workerSettlementRuntime) GetTopicSubscriptions() []eventcatalog.TopicSubscription {
	return []eventcatalog.TopicSubscription{{TopicName: r.topic, EventTypes: []string{"known.fail"}}}
}

func (r *workerSettlementRuntime) DispatchEvent(_ context.Context, eventType string, _ []byte) (eventruntime.DispatchResult, error) {
	if eventType == "known.fail" {
		r.failedCalls.Add(1)
		return eventruntime.DispatchResult{}, errors.New("injected worker dispatch failure")
	}
	r.unknownCalls.Add(1)
	return eventruntime.DispatchResult{Outcome: eventruntime.DispatchUnknown}, nil
}

type workerSettlementObserver struct {
	unknownAcked         atomic.Int32
	unknownPersistFailed atomic.Int32
}

func (*workerSettlementObserver) ObservePublish(context.Context, eventobservability.PublishEvent) {}
func (*workerSettlementObserver) ObserveOutbox(context.Context, eventobservability.OutboxEvent)   {}
func (o *workerSettlementObserver) ObserveConsume(_ context.Context, event eventobservability.ConsumeEvent) {
	if event.Outcome == eventobservability.ConsumeOutcomeUnknownAcked {
		o.unknownAcked.Add(1)
	}
	if event.Outcome == eventobservability.ConsumeOutcomeUnknownPersistFailed {
		o.unknownPersistFailed.Add(1)
	}
}

func TestWorkerSettlementThroughNSQPersistsPoisonUnknownAndExhaustion(t *testing.T) {
	if os.Getenv("MESSAGING_INTEGRATION") != "1" {
		t.Skip("set MESSAGING_INTEGRATION=1 and start NSQ/MySQL integration services")
	}
	db := openIsolatedDeadLetterDatabase(t)
	recorder, err := NewSQLDeadLetterRecorder(db)
	if err != nil {
		t.Fatal(err)
	}
	topic := fmt.Sprintf("qs-worker-settlement-%d", time.Now().UnixNano())
	channel := topic + "-worker"
	cleanupNSQTopics(t, topic, nsqFailedHandoffTopic(topic, channel))
	createNSQTopicAndChannel(t, topic, channel)

	runtime := &workerSettlementRuntime{topic: topic}
	observer := &workerSettlementObserver{}
	subscriber, err := NewSubscriber(SubscriberConfig{
		Provider: "nsq", NSQLookupdAddr: integrationEnv("NSQ_LOOKUPD_ADDR", "127.0.0.1:4161"), NSQMessageTimeout: time.Minute,
	}, basemessaging.SubscriberOptions{
		MaxInFlight: 1, MaxAttempts: 2,
		RetryBackoff:         basemessaging.RetryBackoffOptions{BaseDelay: 10 * time.Millisecond, MaxDelay: 20 * time.Millisecond},
		FailedMessageHandler: FailedMessageHandler(recorder),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscriber.Close() })
	if err := workermessaging.SubscribeHandlersWithOptions(workermessaging.SubscribeHandlersOptions{
		ServiceName: channel, Logger: slog.Default(), Runtime: runtime, Subscriber: subscriber, Observer: observer,
		UnknownRecorder: NewUnknownEventRecorder("nsq", recorder),
	}); err != nil {
		t.Fatal(err)
	}
	publisher, err := cbnsq.NewPublisher(integrationEnv("NSQD_ADDR", "127.0.0.1:4150"), nsq.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = publisher.Close() })

	poison := basemessaging.NewMessage("worker-poison-1", []byte("not-json"))
	unknown := basemessaging.NewMessage("worker-unknown-1", []byte(`{"id":"unknown-1"}`))
	unknown.Metadata["event_type"] = "new.event"
	failed := basemessaging.NewMessage("worker-failed-1", []byte(`{"id":"failed-1"}`))
	failed.Metadata["event_type"] = "known.fail"
	for _, message := range []*basemessaging.Message{poison, unknown, failed} {
		if err := publisher.PublishMessage(t.Context(), topic, message); err != nil {
			t.Fatal(err)
		}
	}

	deadline := time.NewTimer(20 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		var deadLetters int
		if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM event_delivery_dead_letter`).Scan(&deadLetters); err != nil {
			t.Fatal(err)
		}
		if deadLetters == 3 && observer.unknownAcked.Load() == 1 {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("worker settlement timed out: dead_letters=%d unknown_acked=%d", deadLetters, observer.unknownAcked.Load())
		}
	}
	for _, test := range []struct {
		id      string
		payload string
		cause   string
	}{
		{poison.UUID, string(poison.Payload), "failed to parse event envelope"},
		{failed.UUID, string(failed.Payload), "injected worker dispatch failure"},
	} {
		var payload, cause, disposition string
		var attempts int
		if err := db.QueryRowContext(t.Context(), `SELECT payload_json,last_error,retry_disposition,delivery_attempts
FROM event_delivery_dead_letter WHERE message_id=?`, test.id).Scan(&payload, &cause, &disposition, &attempts); err != nil {
			t.Fatal(err)
		}
		if payload != test.payload || !strings.Contains(cause, test.cause) || disposition != "manual_required" || attempts != 2 {
			t.Fatalf("dead letter %q: payload=%q cause=%q disposition=%q attempts=%d", test.id, payload, cause, disposition, attempts)
		}
	}
	var unknownEventID, unknownPayload, unknownCause, unknownDisposition string
	var unknownAttempts int
	if err := db.QueryRowContext(t.Context(), `SELECT event_id,payload_json,last_error,retry_disposition,delivery_attempts FROM event_delivery_dead_letter WHERE message_id=?`, unknown.UUID).
		Scan(&unknownEventID, &unknownPayload, &unknownCause, &unknownDisposition, &unknownAttempts); err != nil {
		t.Fatal(err)
	}
	if unknownEventID != "unknown-1" || unknownPayload != string(unknown.Payload) || unknownCause != "unknown event type: new.event" || unknownDisposition != "manual_required" || unknownAttempts != 1 || runtime.unknownCalls.Load() != 1 || runtime.failedCalls.Load() != 2 {
		t.Fatalf("unknown audit: event=%q payload=%q cause=%q disposition=%q attempts=%d calls=%d/%d", unknownEventID, unknownPayload, unknownCause, unknownDisposition, unknownAttempts, runtime.unknownCalls.Load(), runtime.failedCalls.Load())
	}
}

func TestNSQDeliveryExhaustionPersistsMySQLDeadLetter(t *testing.T) {
	if os.Getenv("MESSAGING_INTEGRATION") != "1" {
		t.Skip("set MESSAGING_INTEGRATION=1 and start NSQ/MySQL integration services")
	}
	db := openIsolatedDeadLetterDatabase(t)
	recorder, err := NewSQLDeadLetterRecorder(db)
	if err != nil {
		t.Fatal(err)
	}

	lookupd := integrationEnv("NSQ_LOOKUPD_ADDR", "127.0.0.1:4161")
	nsqdAddress := integrationEnv("NSQD_ADDR", "127.0.0.1:4150")
	topic := fmt.Sprintf("qs-server-dead-letter-%d", time.Now().UnixNano())
	channel := topic + "-worker"
	cleanupNSQTopics(t, topic, nsqFailedHandoffTopic(topic, channel))
	createNSQTopicAndChannel(t, topic, channel)

	var handlerCalls atomic.Int32
	options := basemessaging.SubscriberOptions{
		MaxInFlight: 1,
		MaxAttempts: 2,
		RetryBackoff: basemessaging.RetryBackoffOptions{
			BaseDelay: 10 * time.Millisecond,
			MaxDelay:  20 * time.Millisecond,
		},
		FailedMessageHandler: FailedMessageHandler(recorder),
	}
	subscriber, err := NewSubscriber(SubscriberConfig{Provider: "nsq", NSQLookupdAddr: lookupd, NSQMessageTimeout: time.Minute}, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscriber.Close() })
	wantCause := errors.New("qs-server transport integration handler failed")
	if err := subscriber.Subscribe(topic, channel, func(context.Context, *basemessaging.Message) error {
		handlerCalls.Add(1)
		return wantCause
	}); err != nil {
		t.Fatal(err)
	}

	publisher, err := cbnsq.NewPublisher(nsqdAddress, nsq.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	payload := []byte(`{"id":"transport-event-1","data":{"org_id":7}}`)
	message := basemessaging.NewMessage("transport-message-1", payload)
	if err := publisher.PublishMessage(t.Context(), topic, message); err != nil {
		t.Fatal(err)
	}

	type deadLetterRow struct {
		MessageID        string
		EventID          string
		OrgID            int64
		Provider         string
		Topic            string
		Channel          string
		DeliveryAttempts int
		Payload          string
		LastError        string
		Disposition      string
	}
	var row deadLetterRow
	deadline := time.NewTimer(20 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		err = db.QueryRowContext(t.Context(), `SELECT message_id,event_id,org_id,provider,topic_name,channel_name,
delivery_attempts,payload_json,last_error,retry_disposition FROM event_delivery_dead_letter WHERE message_id=?`, message.UUID).
			Scan(&row.MessageID, &row.EventID, &row.OrgID, &row.Provider, &row.Topic, &row.Channel, &row.DeliveryAttempts, &row.Payload, &row.LastError, &row.Disposition)
		if err == nil {
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatal(err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("timed out waiting for NSQ terminal handoff to persist in MySQL")
		}
	}

	if got := handlerCalls.Load(); got != 2 {
		t.Fatalf("handler calls = %d, want 2", got)
	}
	if row.MessageID != message.UUID || row.EventID != "transport-event-1" || row.OrgID != 7 || row.Provider != "nsq" || row.Topic != topic || row.Channel != channel || row.DeliveryAttempts != 2 || row.Payload != string(payload) || row.LastError != wantCause.Error() || row.Disposition != "manual_required" {
		t.Fatalf("dead-letter row = %#v", row)
	}
}

func TestWorkerUnknownEventRecoversAfterSubscriberRestart(t *testing.T) {
	if os.Getenv("MESSAGING_INTEGRATION") != "1" {
		t.Skip("set MESSAGING_INTEGRATION=1 and start NSQ/MySQL integration services")
	}
	db := openIsolatedDeadLetterDatabase(t)
	recorder, err := NewSQLDeadLetterRecorder(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `DROP TABLE event_delivery_dead_letter`); err != nil {
		t.Fatal(err)
	}
	failedWrites := make(chan error, 2)
	wrappedRecorder := deadLetterRecorderFunc(func(ctx context.Context, record DeadLetterRecord) error {
		writeErr := recorder.RecordDeadLetter(ctx, record)
		if writeErr != nil {
			select {
			case failedWrites <- writeErr:
			default:
			}
		}
		return writeErr
	})
	topic := fmt.Sprintf("qs-worker-unknown-%d", time.Now().UnixNano())
	channel := topic + "-worker"
	cleanupNSQTopics(t, topic, nsqFailedHandoffTopic(topic, channel))
	createNSQTopicAndChannel(t, topic, channel)
	runtime := &workerSettlementRuntime{topic: topic}
	observer := &workerSettlementObserver{}
	subscriberConfig := SubscriberConfig{
		Provider: "nsq", NSQLookupdAddr: integrationEnv("NSQ_LOOKUPD_ADDR", "127.0.0.1:4161"), NSQMessageTimeout: time.Minute,
	}
	subscriberOptions := basemessaging.SubscriberOptions{
		MaxInFlight: 1, MaxAttempts: 4,
		RetryBackoff:         basemessaging.RetryBackoffOptions{BaseDelay: 500 * time.Millisecond, MaxDelay: 500 * time.Millisecond},
		FailedMessageHandler: FailedMessageHandler(wrappedRecorder),
	}
	subscriber, err := NewSubscriber(subscriberConfig, subscriberOptions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscriber.Close() })
	if err := workermessaging.SubscribeHandlersWithOptions(workermessaging.SubscribeHandlersOptions{
		ServiceName: channel, Logger: slog.Default(), Runtime: runtime, Subscriber: subscriber, Observer: observer,
		UnknownRecorder: NewUnknownEventRecorder("nsq", wrappedRecorder),
	}); err != nil {
		t.Fatal(err)
	}
	publisher, err := cbnsq.NewPublisher(integrationEnv("NSQD_ADDR", "127.0.0.1:4150"), nsq.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	message := basemessaging.NewMessage("worker-unknown-outage-1", []byte(`{"id":"unknown-outage-1","data":{"org_id":501}}`))
	message.Metadata["event_type"] = "future.event"
	if err := publisher.PublishMessage(t.Context(), topic, message); err != nil {
		t.Fatal(err)
	}
	select {
	case writeErr := <-failedWrites:
		if !strings.Contains(writeErr.Error(), "event_delivery_dead_letter") {
			t.Fatalf("unexpected audit failure: %v", writeErr)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for unknown-event audit failure")
	}
	if observer.unknownAcked.Load() != 0 {
		t.Fatalf("unknown event confirmed before durable record: acked=%d", observer.unknownAcked.Load())
	}
	// Stop the first subscriber while the message is still unconfirmed. The
	// replacement must recover it from the same NSQ channel after MySQL returns.
	if err := subscriber.Close(); err != nil {
		t.Fatalf("stop failed Worker subscriber: %v", err)
	}
	if observer.unknownAcked.Load() != 0 {
		t.Fatalf("failed Worker confirmed unknown event during shutdown: acked=%d", observer.unknownAcked.Load())
	}
	if err := createDeadLetterTable(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	restartedObserver := &workerSettlementObserver{}
	restartedSubscriber, err := NewSubscriber(subscriberConfig, subscriberOptions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedSubscriber.Close() })
	if err := workermessaging.SubscribeHandlersWithOptions(workermessaging.SubscribeHandlersOptions{
		ServiceName: channel, Logger: slog.Default(), Runtime: runtime, Subscriber: restartedSubscriber, Observer: restartedObserver,
		UnknownRecorder: NewUnknownEventRecorder("nsq", wrappedRecorder),
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(20 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		var rows, attempts int
		var cause string
		err := db.QueryRowContext(t.Context(), `SELECT COUNT(*),COALESCE(MAX(delivery_attempts),0),COALESCE(MAX(last_error),'') FROM event_delivery_dead_letter WHERE message_id=?`, message.UUID).Scan(&rows, &attempts, &cause)
		if err != nil {
			t.Fatal(err)
		}
		if rows == 1 && restartedObserver.unknownAcked.Load() == 1 {
			if attempts < 2 || attempts >= 4 || cause != "unknown event type: future.event" {
				t.Fatalf("recovered unknown audit: attempts=%d cause=%q", attempts, cause)
			}
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("unknown event audit did not recover after Worker restart: rows=%d acked=%d", rows, restartedObserver.unknownAcked.Load())
		}
	}
	if runtime.unknownCalls.Load() < 2 || runtime.failedCalls.Load() != 0 || observer.unknownPersistFailed.Load() == 0 {
		t.Fatalf("dispatch calls after audit recovery: unknown=%d failed=%d persist_failed=%d", runtime.unknownCalls.Load(), runtime.failedCalls.Load(), observer.unknownPersistFailed.Load())
	}
}

type deadLetterRecorderFunc func(context.Context, DeadLetterRecord) error

func (f deadLetterRecorderFunc) RecordDeadLetter(ctx context.Context, record DeadLetterRecord) error {
	return f(ctx, record)
}

func TestNSQFailedHandoffWaitsForMySQLRecoveryWithoutBusinessRetry(t *testing.T) {
	if os.Getenv("MESSAGING_INTEGRATION") != "1" {
		t.Skip("set MESSAGING_INTEGRATION=1 and start NSQ/MySQL integration services")
	}
	db := openIsolatedDeadLetterDatabase(t)
	recorder, err := NewSQLDeadLetterRecorder(db)
	if err != nil {
		t.Fatal(err)
	}
	// Break only this test's disposable audit table, after its schema has been created.
	if _, err := db.ExecContext(t.Context(), `DROP TABLE event_delivery_dead_letter`); err != nil {
		t.Fatal(err)
	}
	topic := fmt.Sprintf("qs-worker-handoff-%d", time.Now().UnixNano())
	channel := topic + "-worker"
	cleanupNSQTopics(t, topic, nsqFailedHandoffTopic(topic, channel))
	createNSQTopicAndChannel(t, topic, channel)

	failedWrites := make(chan error, 4)
	var businessCalls atomic.Int32
	var auditCalls atomic.Int32
	subscriber, err := NewSubscriber(SubscriberConfig{
		Provider: "nsq", NSQLookupdAddr: integrationEnv("NSQ_LOOKUPD_ADDR", "127.0.0.1:4161"), NSQMessageTimeout: time.Minute,
	}, basemessaging.SubscriberOptions{
		MaxInFlight: 1, MaxAttempts: 2,
		RetryBackoff: basemessaging.RetryBackoffOptions{BaseDelay: 100 * time.Millisecond, MaxDelay: 100 * time.Millisecond},
		FailedMessageHandler: FailedMessageHandler(deadLetterRecorderFunc(func(ctx context.Context, record DeadLetterRecord) error {
			auditCalls.Add(1)
			writeErr := recorder.RecordDeadLetter(ctx, record)
			if writeErr != nil {
				select {
				case failedWrites <- writeErr:
				default:
				}
			}
			return writeErr
		})),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscriber.Close() })
	if err := subscriber.Subscribe(topic, channel, func(context.Context, *basemessaging.Message) error {
		businessCalls.Add(1)
		return errors.New("injected business failure before audit outage")
	}); err != nil {
		t.Fatal(err)
	}
	publisher, err := cbnsq.NewPublisher(integrationEnv("NSQD_ADDR", "127.0.0.1:4150"), nsq.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	message := basemessaging.NewMessage("worker-handoff-outage-1", []byte(`{"id":"handoff-1","data":{"org_id":501}}`))
	if err := publisher.PublishMessage(t.Context(), topic, message); err != nil {
		t.Fatal(err)
	}
	select {
	case writeErr := <-failedWrites:
		if !strings.Contains(writeErr.Error(), "event_delivery_dead_letter") {
			t.Fatalf("unexpected audit failure: %v", writeErr)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for the failed-message handoff to reach unavailable MySQL")
	}
	if got := businessCalls.Load(); got != 2 {
		t.Fatalf("business calls after audit failure = %d, want 2", got)
	}
	if err := createDeadLetterTable(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(20 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		var attempts int
		var cause string
		err := db.QueryRowContext(t.Context(), `SELECT delivery_attempts,last_error FROM event_delivery_dead_letter WHERE message_id=?`, message.UUID).Scan(&attempts, &cause)
		if err == nil {
			if attempts != 2 || cause != "injected business failure before audit outage" {
				t.Fatalf("recovered audit: attempts=%d cause=%q", attempts, cause)
			}
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatal(err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("timed out waiting for MySQL audit recovery")
		}
	}
	if got := businessCalls.Load(); got != 2 {
		t.Fatalf("business handler reentered during handoff recovery: %d calls", got)
	}
	if got := auditCalls.Load(); got < 2 {
		t.Fatalf("audit writes = %d, want failure then retry", got)
	}
}

func openIsolatedDeadLetterDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN is required for transport integration tests")
	}
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := fmt.Sprintf("qs_transport_%d", time.Now().UnixNano())
	cfg.DBName = ""
	server, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	if _, err := server.ExecContext(t.Context(), "CREATE DATABASE `"+databaseName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = server.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+databaseName+"`") })
	cfg.DBName = databaseName
	cfg.ParseTime = true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := createDeadLetterTable(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func createDeadLetterTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE event_delivery_dead_letter (
 id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
 message_id varchar(128) NOT NULL,
 event_id varchar(128) NULL,
 org_id bigint NULL,
 provider varchar(32) NOT NULL,
 topic_name varchar(255) NOT NULL,
 channel_name varchar(255) NOT NULL,
 delivery_attempts int NOT NULL,
 payload_json longtext NOT NULL,
 last_error text NULL,
 retry_disposition varchar(32) NOT NULL,
 failed_at datetime(3) NOT NULL,
 created_at datetime(3) NOT NULL,
 updated_at datetime(3) NOT NULL,
 UNIQUE KEY uq_delivery_identity (provider,topic_name,channel_name,message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	return err
}

func nsqFailedHandoffTopic(topic, channel string) string {
	digest := sha256.Sum256([]byte(topic + "\x00" + channel))
	return fmt.Sprintf("cb.failed.%x", digest[:12])
}

func cleanupNSQTopics(t *testing.T, topics ...string) {
	t.Helper()
	httpAddress := integrationEnv("NSQD_HTTP_ADDR", "127.0.0.1:4151")
	lookupAddress := integrationEnv("NSQ_LOOKUPD_ADDR", "127.0.0.1:4161")
	t.Cleanup(func() {
		for _, topic := range topics {
			response, err := postNSQAdmin(httpAddress, "/topic/delete?topic="+url.QueryEscape(topic))
			if err != nil {
				t.Errorf("delete NSQ topic %q: %v", topic, err)
				continue
			}
			_ = response.Body.Close()
			if response.StatusCode != http.StatusNotFound && (response.StatusCode < 200 || response.StatusCode >= 300) {
				t.Errorf("delete NSQ topic %q: status %s", topic, response.Status)
			}
			lookupResponse, lookupErr := postNSQAdmin(lookupAddress, "/topic/delete?topic="+url.QueryEscape(topic))
			if lookupErr != nil {
				t.Errorf("delete NSQ lookup registration %q: %v", topic, lookupErr)
				continue
			}
			_ = lookupResponse.Body.Close()
			if lookupResponse.StatusCode != http.StatusNotFound && (lookupResponse.StatusCode < 200 || lookupResponse.StatusCode >= 300) {
				t.Errorf("delete NSQ lookup registration %q: status %s", topic, lookupResponse.Status)
			}
		}
	})
}

func createNSQTopicAndChannel(t *testing.T, topic, channel string) {
	t.Helper()
	httpAddress := integrationEnv("NSQD_HTTP_ADDR", "127.0.0.1:4151")
	for _, endpoint := range []string{
		"/topic/create?topic=" + url.QueryEscape(topic),
		"/channel/create?topic=" + url.QueryEscape(topic) + "&channel=" + url.QueryEscape(channel),
	} {
		response, err := postNSQAdmin(httpAddress, endpoint)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatalf("prepare NSQ topic/channel: status %s", response.Status)
		}
	}
}

func postNSQAdmin(address, path string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://"+address+path, nil)
	if err != nil {
		return nil, err
	}
	return (&http.Client{Timeout: 5 * time.Second}).Do(request)
}

func integrationEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
