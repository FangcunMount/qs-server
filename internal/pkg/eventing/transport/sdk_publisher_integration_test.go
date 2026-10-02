//go:build integration

package transport

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
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

	received := make(chan rmtransport.Received, 1)
	subscriber, err := NewSDKDeliverySubscriber(SubscriberConfig{
		Provider: "nsq", NSQLookupdAddr: integrationEnv("NSQ_LOOKUPD_ADDR", "127.0.0.1:4161"),
	}, 1, 2, func(_ context.Context, failed legacy.FailedHandoff) error {
		return fmt.Errorf("unexpected failed handoff for %s", failed.Topic)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { subscriber.Stop(); _ = subscriber.Close() })
	if err := subscriber.Subscribe(topic, channel, func(_ context.Context, delivery rmtransport.Delivery) error {
		received <- delivery.Message()
		return delivery.Ack()
	}); err != nil {
		t.Fatal(err)
	}

	publisher, err := messagingruntime.NewSDKNSQWirePublisher(integrationEnv("NSQD_ADDR", "127.0.0.1:4150"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	original := legacy.Envelope{UUID: "qs-event-identity-1", Metadata: map[string]string{}, Payload: []byte(`{"id":"qs-event-identity-1"}`)}
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
		if msg.ID != original.UUID || msg.TransportID == "" || msg.TransportID == msg.ID ||
			string(msg.Payload) != string(original.Payload) || msg.Metadata["event_type"] != original.Metadata["event_type"] {
			t.Fatalf("SDK publish/subscribe changed QS delivery identity or payload: application=%q physical=%q metadata=%v payload=%q", msg.ID, msg.TransportID, msg.Metadata, msg.Payload)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for SDK-published QS event")
	}
}
