//go:build integration

package aibridge

import (
	"bytes"
	"context"
	"database/sql"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/google/uuid"
)

// Exercise the actual duplicate receiver and host relay, rather than invoking
// SDK settlement directly: a committed duplicate must requeue the original ACK
// without exhausting its failure budget or reapplying the business projection.
func TestMQFinalAckRepeatedSuccessRetainsBudgetAndOriginalProjection(t *testing.T) {
	f := newMQFixture(t)
	mustMQ(t, seedLegacyStartFixture(t.Context(), f.request, &Store{DB: f.db}))
	id := uuid.NewString()
	message := f.protect(pb.MessagingKind_INTERPRETATION_STATE, id, f.request.RequestID, "", &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: &pb.StateEvent{EventId: id, RequestId: f.request.RequestID, SessionId: f.session, Actor: &pb.Actor{OrgId: f.scope.OrganizationID, SubjectId: f.scope.SubjectID}, TesteeId: f.request.TesteeID, Version: 1, Status: "queued"}}}, true)
	receiver := receiverFixture(f, &localEventBody{})
	mustMQ(t, receiver.Receive(t.Context(), message.Wire))
	var ackID string
	mustMQ(t, f.db.QueryRow("SELECT ack_id FROM ai_messaging_inbox WHERE message_id=?", id).Scan(&ackID))
	var originalWire, originalBody []byte
	var originalCreated time.Time
	mustMQ(t, f.db.QueryRow("SELECT wire,body,created_at FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&originalWire, &originalBody, &originalCreated))
	calls := 0
	relay, err := NewMessagingRelay(f.db, f.store.Outbox, mqPublisherFunc(func(_ context.Context, topic string, wire []byte) transport.Result {
		calls++
		if topic != "qs.ai.acks.v1" || !bytes.Equal(wire, originalWire) {
			t.Fatal("duplicate resealed the original final ACK")
		}
		if calls <= 2 {
			return transport.Result{Outcome: transport.Unknown}
		}
		return transport.Result{Outcome: transport.Confirmed}
	}))
	mustMQ(t, err)
	for i := 0; i < 2; i++ {
		mustMQ(t, relay.Step(t.Context()))
		_, err := f.db.Exec("UPDATE ai_messaging_outbox SET available_at=UTC_TIMESTAMP(6) WHERE message_id=?", ackID)
		mustMQ(t, err)
	}
	for i := 0; i < 16; i++ {
		mustMQ(t, relay.Step(t.Context()))
		var stage string
		var attempts uint64
		var body, wire []byte
		var created time.Time
		mustMQ(t, f.db.QueryRow("SELECT stage,attempts,body,wire,created_at FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&stage, &attempts, &body, &wire, &created))
		if stage != "confirmed" || attempts != 2 || !bytes.Equal(body, originalBody) || !bytes.Equal(wire, originalWire) || !created.Equal(originalCreated) {
			t.Fatal("successful duplicate ACK spent failure budget or changed original identity")
		}
		// A fresh host receiver stands for process-local state loss; the persisted
		// first ACK and projection still determine the duplicate response.
		if i != 15 {
			receiver = receiverFixture(f, &localEventBody{})
			mustMQ(t, receiver.Receive(t.Context(), message.Wire))
		}
	}
	if calls != 18 {
		t.Fatal("final ACK was held before sixteen successful retransmissions")
	}
	var projectionCount int
	mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_bridge_events WHERE event_id=?", id).Scan(&projectionCount))
	if projectionCount != 1 {
		t.Fatal("duplicate applied another business projection")
	}
	mustMQ(t, f.db.Ping())
}

func TestMQHeldFinalAckDuplicateCannotRenewBudgetOrPublish(t *testing.T) {
	f := newMQFixture(t)
	mustMQ(t, seedLegacyStartFixture(t.Context(), f.request, &Store{DB: f.db}))
	id := uuid.NewString()
	message := f.protect(pb.MessagingKind_INTERPRETATION_STATE, id, f.request.RequestID, "", &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: &pb.StateEvent{EventId: id, RequestId: f.request.RequestID, SessionId: f.session, Actor: &pb.Actor{OrgId: f.scope.OrganizationID, SubjectId: f.scope.SubjectID}, TesteeId: f.request.TesteeID, Version: 1, Status: "queued"}}}, true)
	receiver := receiverFixture(f, &localEventBody{})
	mustMQ(t, receiver.Receive(t.Context(), message.Wire))
	var ackID string
	mustMQ(t, f.db.QueryRow("SELECT ack_id FROM ai_messaging_inbox WHERE message_id=?", id).Scan(&ackID))
	calls := 0
	relay, err := NewMessagingRelay(f.db, f.store.Outbox, mqPublisherFunc(func(context.Context, string, []byte) transport.Result {
		calls++
		return transport.Result{Outcome: transport.Unknown}
	}))
	mustMQ(t, err)
	for i := 0; i < 8; i++ {
		mustMQ(t, relay.Step(t.Context()))
		mustMQ(t, f.tx(func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE ai_messaging_outbox SET available_at=UTC_TIMESTAMP(6) WHERE message_id=?", ackID)
			return err
		}))
	}
	mustMQ(t, relay.Step(t.Context()))
	for i := 0; i < 3; i++ {
		mustMQ(t, receiver.Receive(t.Context(), message.Wire))
		var retainedStage string
		mustMQ(t, f.db.QueryRow("SELECT stage FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&retainedStage))
		if retainedStage != "held" {
			t.Fatal("committed duplicate removed the existing technical hold")
		}
		mustMQ(t, relay.Step(t.Context()))
	}
	var stage, code string
	var attempts uint64
	mustMQ(t, f.db.QueryRow("SELECT stage,error_code,attempts FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&stage, &code, &attempts))
	if stage != "held" || code != "publish_budget_exhausted" || attempts != 8 || calls != 8 {
		t.Fatal("duplicate granted a new final ACK publication budget")
	}
}
