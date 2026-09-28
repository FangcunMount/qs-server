//go:build integration

package transport

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	workermessaging "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	rmnsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/nsqio/go-nsq"
)

func TestSDKWorkerDeliveryPersistsPoisonUnknownAndExhaustion(t *testing.T) {
	if os.Getenv("MESSAGING_INTEGRATION") != "1" {
		t.Skip("set MESSAGING_INTEGRATION=1 and start NSQ/MySQL integration services")
	}
	db := openIsolatedDeadLetterDatabase(t)
	recorder, err := NewSQLDeadLetterRecorder(db)
	if err != nil {
		t.Fatal(err)
	}
	topic := fmt.Sprintf("qs-sdk-worker-%d", time.Now().UnixNano())
	channel := topic + "-worker"
	cleanupNSQTopics(t, topic, nsqFailedHandoffTopic(topic, channel))
	createNSQTopicAndChannel(t, topic, channel)

	subscriber, err := NewSDKDeliverySubscriber(SubscriberConfig{
		Provider: "nsq", NSQLookupdAddr: integrationEnv("NSQ_LOOKUPD_ADDR", "127.0.0.1:4161"), NSQMessageTimeout: time.Minute,
	}, 1, 1, SDKFailedHandoffHandler(recorder))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscriber.Close() })
	runtime := &workerSettlementRuntime{topic: topic}
	observer := &workerSettlementObserver{}
	if err := workermessaging.SubscribeSDKHandlersWithOptions(workermessaging.SubscribeSDKHandlersOptions{
		ServiceName: channel, Logger: slog.Default(), Runtime: runtime, Subscriber: subscriber, Observer: observer,
		UnknownRecorder: NewDeliveryUnknownEventRecorder("nsq", recorder),
	}); err != nil {
		t.Fatal(err)
	}
	producer, err := nsq.NewProducer(integrationEnv("NSQD_ADDR", "127.0.0.1:4150"), nsq.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(producer.Stop)
	if err := producer.Ping(); err != nil {
		t.Fatal(err)
	}
	publisher, err := rmnsq.New(producer, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id, eventType, payload string
	}{
		{"sdk-poison-1", "", "not-json"},
		{"sdk-unknown-1", "future.event", `{"id":"unknown-1"}`},
		{"sdk-failed-1", "known.fail", `{"id":"failed-1"}`},
	} {
		metadata := map[string]string{}
		if item.eventType != "" {
			metadata["event_type"] = item.eventType
		}
		wire, err := legacy.Encode(legacy.Envelope{UUID: item.id, Metadata: metadata, Payload: []byte(item.payload)}, legacy.Revision2)
		if err != nil {
			t.Fatal(err)
		}
		if result := publisher.PublishRaw(t.Context(), topic, wire); result.Outcome != rmtransport.Confirmed {
			t.Fatalf("publish %s outcome = %d", item.id, result.Outcome)
		}
	}
	deadline := time.NewTimer(20 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		var rows int
		if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM event_delivery_dead_letter`).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows == 3 && observer.unknownAcked.Load() == 1 {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("SDK Worker settlement timed out: dead_letters=%d unknown_acked=%d", rows, observer.unknownAcked.Load())
		}
	}
	for _, item := range []struct{ id, cause string }{
		{"sdk-poison-1", "failed to parse event envelope"},
		{"sdk-failed-1", "injected worker dispatch failure"},
		{"sdk-unknown-1", "unknown event type: future.event"},
	} {
		var physicalID, cause, disposition string
		var attempts int
		if err := db.QueryRowContext(t.Context(), `SELECT transport_message_id,last_error,retry_disposition,delivery_attempts
FROM event_delivery_dead_letter WHERE message_id=?`, item.id).Scan(&physicalID, &cause, &disposition, &attempts); err != nil {
			t.Fatal(err)
		}
		if physicalID == "" || !strings.Contains(cause, item.cause) || disposition != "manual_required" || attempts != 1 {
			t.Fatalf("SDK Worker audit %s: physical=%q cause=%q disposition=%q attempts=%d", item.id, physicalID, cause, disposition, attempts)
		}
	}
	if runtime.failedCalls.Load() != 1 || runtime.unknownCalls.Load() != 1 {
		t.Fatalf("Worker dispatch calls: failed=%d unknown=%d", runtime.failedCalls.Load(), runtime.unknownCalls.Load())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := publisher.Drain(ctx); err != nil {
		t.Fatal(err)
	}
}
