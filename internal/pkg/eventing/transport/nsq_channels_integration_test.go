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
	original := legacy.Envelope{UUID: "first-application-event", Metadata: map[string]string{}, Payload: []byte(`{"id":"first-application-event"}`)}
	wire, err := legacy.Encode(legacy.Envelope{UUID: original.UUID, Metadata: original.Metadata, Payload: original.Payload}, legacy.Revision2)
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.PublishWire(t.Context(), topic, wire); err != nil {
		t.Fatal(err)
	}

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
	select {
	case message := <-received:
		if message.ID != original.UUID || string(message.Payload) != string(original.Payload) {
			t.Fatalf("first event changed: application=%q payload=%q", message.ID, message.Payload)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("first event was lost before subscriber started")
	}
}
