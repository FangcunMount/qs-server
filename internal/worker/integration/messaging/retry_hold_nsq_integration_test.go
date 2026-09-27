//go:build reliable_messaging

package messaging

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	driver "github.com/nsqio/go-nsq"
	"github.com/stretchr/testify/require"
)

func TestRetryHoldReplayerUsesRealNSQWire(t *testing.T) {
	address := os.Getenv("RM_QS_NSQ_TCP")
	if address != "nsqd:4150" {
		t.Fatal("disposable nsqd:4150 required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	topic := fmt.Sprintf("qs-m6-hold-%d", time.Now().UnixNano())
	consumer, err := driver.NewConsumer(topic, "wire-proof", driver.NewConfig())
	require.NoError(t, err)
	consumer.SetLogger(nil, driver.LogLevelError)
	received := make(chan []byte, 1)
	consumer.AddHandler(driver.HandlerFunc(func(message *driver.Message) error {
		received <- append([]byte(nil), message.Body...)
		return nil
	}))
	require.NoError(t, consumer.ConnectToNSQD(address))
	defer func() {
		consumer.Stop()
		select {
		case <-consumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("retry hold proof consumer did not stop")
		}
	}()
	publisher, err := messagingruntime.NewSDKNSQPublisher(address)
	require.NoError(t, err)
	defer func() { require.NoError(t, publisher.Close()) }()
	item := &heldEvent{ID: 1, EventID: "original-event", MessageID: "original-message", Topic: topic, Payload: []byte(`{"id":"original-event","eventType":"evaluation.retry.requested"}`), ClaimToken: "proof-claim"}
	store := &holdStoreStub{items: []*heldEvent{item}}
	replayer, err := NewRetryEventHoldReplayerForProvider(store, publisher, "nsq")
	require.NoError(t, err)
	require.NoError(t, replayer.RunOnce(ctx, time.Now()))
	require.Equal(t, 1, store.replayed)
	want, err := legacy.Encode(legacy.Envelope{UUID: item.MessageID, Payload: item.Payload}, legacy.Revision2)
	require.NoError(t, err)
	select {
	case raw := <-received:
		require.True(t, bytes.Equal(want, raw), "replayed message lost its original NSQ wire identity")
	case <-ctx.Done():
		t.Fatal("replayed event did not reach NSQ", ctx.Err())
	}
}
