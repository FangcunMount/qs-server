//go:build integration

package aibridge

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	"github.com/FangcunMount/reliable-messaging/transport"
)

type mqPublisherFunc func(context.Context, string, []byte) transport.Result

func (f mqPublisherFunc) PublishRaw(ctx context.Context, topic string, body []byte) transport.Result {
	return f(ctx, topic, body)
}

func TestMQRelayBrokerAcceptanceRetainsOriginalWire(t *testing.T) {
	f := newMQFixture(t)
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		_, err := f.stage(tx, pb.MessagingKind_START, f.request.RequestID, f.request.RequestID, f.startBody(), f.scope, nil)
		return err
	}))
	var original []byte
	mustMQ(t, f.db.QueryRow("SELECT wire FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&original))
	calls := 0
	relay, err := NewMessagingRelay(f.db, f.store.Outbox, mqPublisherFunc(func(ctx context.Context, topic string, wire []byte) transport.Result {
		calls++
		if topic != "qs.ai.commands.v1" || !bytes.Equal(wire, original) {
			t.Fatal("original wire changed")
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Fatal("unbounded publish")
		}
		return transport.Result{Outcome: transport.Confirmed}
	}))
	mustMQ(t, err)
	mustMQ(t, relay.Step(context.Background()))
	var stage string
	var attempts int
	mustMQ(t, f.db.QueryRow("SELECT stage,attempts FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&stage, &attempts))
	if stage != "awaiting_receipt" || attempts != 1 || calls != 1 {
		t.Fatal("broker confirmation became business confirmation", stage, attempts, calls)
	}
	mustMQ(t, f.db.Ping())
	mustMQ(t, relay.Step(context.Background()))
	if calls != 1 {
		t.Fatal("receipt wait ignored")
	}
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE ai_messaging_outbox SET available_at=UTC_TIMESTAMP(6) WHERE message_id=?", f.request.RequestID)
		return err
	}))
	mustMQ(t, relay.Step(context.Background()))
	if calls != 2 {
		t.Fatal("lost receipt did not re-PUB original")
	}
}

func TestMQRelayCancellationAndOverlapPreservePending(t *testing.T) {
	f := newMQFixture(t)
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		_, err := f.stage(tx, pb.MessagingKind_START, f.request.RequestID, f.request.RequestID, f.startBody(), f.scope, nil)
		return err
	}))
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	relay, err := NewMessagingRelay(f.db, f.store.Outbox, mqPublisherFunc(func(ctx context.Context, _ string, _ []byte) transport.Result {
		close(entered)
		<-ctx.Done()
		return transport.Result{Outcome: transport.Unknown}
	}))
	mustMQ(t, err)
	done := make(chan error, 1)
	go func() { done <- relay.Step(ctx) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("publish did not start")
	}
	if err := relay.Step(context.Background()); !errors.Is(err, ErrMessagingRelayActive) {
		t.Fatal("overlapping relay admitted", err)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation blocked")
	}
	var stage string
	var attempts int
	mustMQ(t, f.db.QueryRow("SELECT stage,attempts FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&stage, &attempts))
	if stage != "staged" || attempts != 0 {
		t.Fatal("cancelled unknown publish settled", stage, attempts)
	}
}

func TestMQRelayPersistentBudgetAndBoundedUnknown(t *testing.T) {
	f := newMQFixture(t)
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		_, err := f.stage(tx, pb.MessagingKind_START, f.request.RequestID, f.request.RequestID, f.startBody(), f.scope, nil)
		return err
	}))
	calls := 0
	relay, err := NewMessagingRelay(f.db, f.store.Outbox, mqPublisherFunc(func(context.Context, string, []byte) transport.Result {
		calls++
		return transport.Result{Outcome: transport.Unknown}
	}))
	mustMQ(t, err)
	for attempt := 0; attempt < 8; attempt++ {
		mustMQ(t, relay.Step(context.Background()))
		var got int
		var delay int
		mustMQ(t, f.db.QueryRow("SELECT attempts,TIMESTAMPDIFF(SECOND,UTC_TIMESTAMP(6),available_at) FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&got, &delay))
		if got != attempt+1 || delay < 0 || delay > 60 {
			t.Fatal("retry budget/delay changed", got, delay)
		}
		_, err := f.db.Exec("UPDATE ai_messaging_outbox SET available_at=UTC_TIMESTAMP(6) WHERE message_id=?", f.request.RequestID)
		mustMQ(t, err)
	}
	mustMQ(t, relay.Step(context.Background()))
	var stage, code string
	mustMQ(t, f.db.QueryRow("SELECT stage,error_code FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&stage, &code))
	if stage != "held" || code != "publish_budget_exhausted" || calls != 8 {
		t.Fatal("new PUB reset persistent budget", stage, code, calls)
	}
	mustMQ(t, relay.Step(context.Background()))
	if calls != 8 {
		t.Fatal("held record published")
	}
}
