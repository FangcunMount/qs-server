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

func TestSDKNSQPublisherRetainsQSApplicationIdentity(t *testing.T) {
	if os.Getenv("MESSAGING_INTEGRATION") != "1" {
		t.Skip("set MESSAGING_INTEGRATION=1 and start NSQ integration services")
	}
	topic := fmt.Sprintf("qs-sdk-publisher-%d", time.Now().UnixNano())
	channel := topic + "-worker"
	cleanupNSQTopics(t, topic, nsqFailedHandoffTopic(topic, channel))
	createNSQTopicAndChannel(t, topic, channel)

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
	if err := subscriber.Subscribe(topic, channel, func(_ context.Context, msg *basemessaging.Message) error {
		received <- msg
		return msg.Ack()
	}); err != nil {
		t.Fatal(err)
	}

	publisher, err := messagingruntime.NewSDKNSQWirePublisher(integrationEnv("NSQD_ADDR", "127.0.0.1:4150"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	original := basemessaging.NewMessage("qs-event-identity-1", []byte(`{"id":"qs-event-identity-1"}`))
	original.Metadata["event_type"] = "assessment.requested"
	wire, err := legacy.Encode(legacy.Envelope{UUID: original.UUID, Metadata: original.Metadata, Payload: original.Payload}, legacy.Revision2)
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.PublishWire(t.Context(), topic, wire); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-received:
		if msg.UUID != original.UUID || msg.TransportMessageID == "" || msg.TransportMessageID == msg.UUID ||
			string(msg.Payload) != string(original.Payload) || msg.Metadata["event_type"] != original.Metadata["event_type"] {
			t.Fatalf("SDK publish/subscribe changed QS delivery identity or payload: application=%q physical=%q metadata=%v payload=%q", msg.UUID, msg.TransportMessageID, msg.Metadata, msg.Payload)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for SDK-published QS event")
	}
}
