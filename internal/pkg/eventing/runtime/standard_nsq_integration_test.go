//go:build reliable_messaging

package eventruntime

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/eventcodec"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transport"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	driver "github.com/nsqio/go-nsq"
	"github.com/stretchr/testify/require"
)

func TestStandardWireThroughRealNSQ(t *testing.T) {
	address := os.Getenv("RM_QS_NSQ_TCP")
	if address != "nsqd:4150" {
		t.Fatal("disposable nsqd:4150 required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	const topic, channel = "qs.evaluation.lifecycle", "qs-m4-wire-proof"
	cfg := driver.NewConfig()
	cfg.HeartbeatInterval = time.Second
	cfg.DialTimeout = 2 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 2 * time.Second
	consumer, err := driver.NewConsumer(topic, channel, cfg)
	require.NoError(t, err)
	consumer.SetLogger(nil, driver.LogLevelError)
	received := make(chan []byte, 1)
	consumer.AddHandler(driver.HandlerFunc(func(raw *driver.Message) error {
		received <- append([]byte(nil), raw.Body...)
		return nil
	}))
	require.NoError(t, consumer.ConnectToNSQD(address))
	defer func() {
		consumer.Stop()
		select {
		case <-consumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("consumer did not stop")
		}
	}()
	producer, err := driver.NewProducer(address, cfg)
	require.NoError(t, err)
	producer.SetLogger(nil, driver.LogLevelError)
	defer producer.Stop()
	publisher, err := sdknsq.New(producer, map[string]string{topic: topic}, 1)
	require.NoError(t, err)
	evt := event.Event[map[string]any]{
		BaseEvent: event.BaseEvent{
			ID: "qs-m4-real-nsq-event", EventTypeValue: "evaluation.requested",
			OccurredAtValue:    time.Date(2026, 9, 23, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)),
			AggregateTypeValue: "Evaluation", AggregateIDValue: "assessment-1",
		},
		Data: map[string]any{"org_id": 1, "assessment_id": 1},
	}
	wire, err := standardoutbox.EncodeWire(evt, SourceAPIServer)
	require.NoError(t, err)
	intent, err := message.New(message.Input{
		Producer: "qs-server", ID: evt.EventID(), Destination: topic,
		EventType: evt.EventType(), SchemaVersion: "1", Scope: "org:1",
		ContentType: "application/json", OccurredAt: evt.OccurredAt().Format(time.RFC3339Nano), Payload: wire,
	})
	require.NoError(t, err)
	require.Equal(t, transport.Confirmed, publisher.Publish(ctx, intent).Outcome)
	require.NoError(t, publisher.Drain(ctx))
	select {
	case raw := <-received:
		require.True(t, bytes.Equal(wire, raw), "NSQ changed standard intent wire bytes")
		decoded, recognized, err := messaging.DecodeMessagePayload(raw)
		require.NoError(t, err)
		require.True(t, recognized)
		require.Equal(t, evt.EventID(), decoded.UUID)
		require.Equal(t, evt.EventType(), decoded.Metadata["event_type"])
		require.Equal(t, SourceAPIServer, decoded.Metadata["source"])
		require.Equal(t, "2026-09-23T10:00:00.000+08:00", decoded.Metadata["occurred_at"])
		envelope, err := eventcodec.DecodeEnvelope(decoded.Payload)
		require.NoError(t, err)
		require.Equal(t, evt.EventID(), envelope.ID)
		require.Equal(t, evt.EventType(), envelope.EventType)
	case <-ctx.Done():
		t.Fatal("real NSQ delivery missing", ctx.Err())
	}
}
