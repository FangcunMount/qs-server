//go:build integration

package transport

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

func TestPreparedNSQChannelRetainsFirstMessageBeforeSubscriberStarts(t *testing.T) {
	if os.Getenv("MESSAGING_INTEGRATION") != "1" {
		t.Skip("set MESSAGING_INTEGRATION=1 and start NSQ integration services")
	}
	topic := fmt.Sprintf("qs-prepared-first-%d", time.Now().UnixNano())
	channel := topic + "-worker"
	cleanupNSQTopics(t, topic, nsqFailedHandoffTopic(topic, channel))
	if err := messagingruntime.EnsureNSQChannels(t.Context(), []string{"http://" + integrationEnv("NSQD_HTTP_ADDR", "127.0.0.1:4151")}, []messagingruntime.DurableChannel{{Topic: topic, Channel: channel}}); err != nil {
		t.Fatal(err)
	}
	publisher, err := messagingruntime.NewSDKNSQWirePublisher(integrationEnv("NSQD_ADDR", "127.0.0.1:4150"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	original := basemessaging.NewMessage("first-application-event", []byte(`{"id":"first-application-event"}`))
	wire, err := legacy.Encode(legacy.Envelope{UUID: original.UUID, Metadata: original.Metadata, Payload: original.Payload}, legacy.Revision2)
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.PublishWire(t.Context(), topic, wire); err != nil {
		t.Fatal(err)
	}

	received := make(chan *basemessaging.Message, 1)
	subscriber, err := newHistoricalNSQSubscriber(SubscriberConfig{
		Provider: "nsq", NSQLookupdAddr: integrationEnv("NSQ_LOOKUPD_ADDR", "127.0.0.1:4161"),
	}, basemessaging.SubscriberOptions{
		MaxInFlight: 1, MaxAttempts: 2,
		FailedMessageHandler: func(_ context.Context, failed basemessaging.FailedMessage) error {
			return fmt.Errorf("unexpected failed handoff for %s", failed.Topic)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { subscriber.Stop(); _ = subscriber.Close() })
	if err := subscriber.Subscribe(topic, channel, func(_ context.Context, message *basemessaging.Message) error {
		received <- message
		return message.Ack()
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-received:
		if message.UUID != original.UUID || string(message.Payload) != string(original.Payload) {
			t.Fatalf("first event changed: application=%q payload=%q", message.UUID, message.Payload)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("first event was lost before subscriber started")
	}
}
