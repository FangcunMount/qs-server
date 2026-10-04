//go:build integration

package aibridge

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

type localEventBody struct{ unavailable bool }

func (b *localEventBody) Read(_ context.Context, e *pb.MessagingEnvelope) ([]byte, error) {
	if b.unavailable {
		return nil, app.ErrManagementUnavailable
	}
	return e.GetInlineBody(), nil
}
func receiverFixture(f *mqFixture, b *localEventBody) *MessagingEventReceiver {
	return &MessagingEventReceiver{DB: f.db, Store: f.store, Bodies: b,
		Keys: protected.Keyring{Decrypt: map[string]jose.JSONWebKey{f.qsCrypt.KeyID: f.qsCrypt}, Signers: map[string]protected.TrustedSigner{f.aiSign.KeyID: {Producer: "qs-ai", Key: f.aiSign.Public()}}},
		Seal: func(k pb.MessagingKind, id, agg, org, original string, body *pb.MessagingBody) (*app.PreparedMessaging, error) {
			return app.ProtectMessaging(k, id, agg, "", org, original, body, f.qsSign, f.aiCrypt.Public())
		},
	}
}
func TestMQReceiverBudgetSurvivesRepublishAndRestartWithoutProjection(t *testing.T) {
	f := newMQFixture(t)
	mustMQ(t, seedLegacyStartFixture(t.Context(), f.request, &Store{DB: f.db}))
	id := uuid.NewString()
	body := &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: &pb.StateEvent{EventId: id, RequestId: f.request.RequestID, SessionId: f.session, Actor: &pb.Actor{OrgId: f.scope.OrganizationID, SubjectId: f.scope.SubjectID}, TesteeId: f.request.TesteeID, Version: 1, Status: "queued"}}}
	m := f.protect(pb.MessagingKind_INTERPRETATION_STATE, id, f.request.RequestID, "", body, true)
	r := receiverFixture(f, &localEventBody{})
	seal := r.Seal
	r.Seal = func(pb.MessagingKind, string, string, string, string, *pb.MessagingBody) (*app.PreparedMessaging, error) {
		return nil, errors.New("injected ACK storage preparation failure")
	}
	for n := 1; n <= 8; n++ {
		if r.Receive(t.Context(), m.Wire) == nil {
			t.Fatal("failure incorrectly confirmed")
		}
		var attempts uint64
		mustMQ(t, f.db.QueryRow("SELECT attempts FROM ai_messaging_failures WHERE message_id=?", id).Scan(&attempts))
		if attempts != uint64(n) {
			t.Fatalf("logical budget %d at %d", attempts, n)
		}
		var events, inbox int
		mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_bridge_events WHERE event_id=?", id).Scan(&events))
		mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_messaging_inbox WHERE message_id=?", id).Scan(&inbox))
		if events != 0 || inbox != 0 {
			t.Fatal("projection/inbox escaped rollback")
		}
	}
	// New receiver represents a restarted process and cannot reset the DB budget.
	r = receiverFixture(f, &localEventBody{})
	r.Seal = seal
	mustMQ(t, r.Receive(t.Context(), m.Wire))
	var outcome, ackID string
	mustMQ(t, f.db.QueryRow("SELECT outcome,ack_id FROM ai_messaging_inbox WHERE message_id=?", id).Scan(&outcome, &ackID))
	if outcome != "held" {
		t.Fatal("technical hold became stored")
	}
	var wire []byte
	mustMQ(t, f.db.QueryRow("SELECT wire FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&wire))
	e, err := app.AuthenticateMessaging(wire, app.AcksTopic, protected.Keyring{Decrypt: map[string]jose.JSONWebKey{f.aiCrypt.KeyID: f.aiCrypt}, Signers: map[string]protected.TrustedSigner{f.qsSign.KeyID: {Producer: "qs-server", Key: f.qsSign.Public()}}})
	mustMQ(t, err)
	parsed, err := app.ParseMessagingBody(e, e.GetInlineBody())
	mustMQ(t, err)
	if parsed.GetEventAcknowledgement().Outcome != pb.MessagingEventAcknowledgement_TECHNICALLY_HELD {
		t.Fatal("false business confirmation")
	}
	mustMQ(t, r.Receive(t.Context(), m.Wire))
	var duplicateWire []byte
	mustMQ(t, f.db.QueryRow("SELECT wire FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&duplicateWire))
	if !bytes.Equal(wire, duplicateWire) {
		t.Fatal("held ACK was recreated")
	}
	var count int
	mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_bridge_events WHERE event_id=?", id).Scan(&count))
	if count != 0 {
		t.Fatal("held event applied a projection")
	}
	mustMQ(t, f.db.Ping()) // borrowed pool is still host-owned
}

func TestMQPhysicalFailureCannotApplyEventOrInventLogicalBudget(t *testing.T) {
	f := newMQFixture(t)
	id := uuid.NewString()
	body := &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationState{EvaluationState: &pb.EvaluationRuntimeState{RunId: f.run, OrganizationId: f.scope.OrganizationID, Version: 1, EventSequence: 1, Status: "requested"}}}
	m := f.protect(pb.MessagingKind_EVALUATION_STATE, id, f.run, "", body, true)
	r := receiverFixture(f, &localEventBody{})
	mustMQ(t, r.Failed(t.Context(), m.Wire))
	var count int
	mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_messaging_failures WHERE message_id=?", id).Scan(&count))
	if count != 0 {
		t.Fatal("physical failure invented logical attempts")
	}
	mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_messaging_inbox WHERE message_id=?", id).Scan(&count))
	if count != 0 {
		t.Fatal("physical failure entered trusted inbox")
	}
	mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_messaging_evaluation_states WHERE run_id=?", f.run).Scan(&count))
	if count != 0 {
		t.Fatal("physical failure applied business state")
	}
	// Durable identity rejects a new body with the same ID; first budget is retained.
	r.Bodies = &localEventBody{unavailable: true}
	if r.Receive(t.Context(), m.Wire) == nil {
		t.Fatal("missing body was acknowledged")
	}
	changed := proto.Clone(body.GetEvaluationState()).(*pb.EvaluationRuntimeState)
	changed.Version = 2
	next := f.protect(pb.MessagingKind_EVALUATION_STATE, id, f.run, "", &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationState{EvaluationState: changed}}, true)
	mustMQ(t, r.Receive(t.Context(), next.Wire))
	var attempts uint64
	mustMQ(t, f.db.QueryRow("SELECT attempts FROM ai_messaging_failures WHERE message_id=?", id).Scan(&attempts))
	if attempts != 1 {
		t.Fatal("conflict altered original budget")
	}
	for _, wire := range [][]byte{m.Wire, next.Wire} {
		_, err := f.db.Exec("DELETE FROM ai_messaging_quarantine WHERE wire_sha256=?", messagingHash(wire))
		mustMQ(t, err)
	}
}

func TestMQReceiverCommitAndCancellationCannotBeBrokerAcknowledged(t *testing.T) {
	f := newMQFixture(t)
	r := receiverFixture(f, &localEventBody{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if r.Receive(ctx, []byte("bad signature")) == nil {
		t.Fatal("cancellation acknowledged")
	}
	// A closed borrowed pool simulates unavailable quarantine storage.
	db, err := sql.Open("mysql", "root@tcp(127.0.0.1:1)/unavailable")
	mustMQ(t, err)
	mustMQ(t, db.Close())
	r.DB = db
	if r.Receive(t.Context(), []byte("bad signature")) == nil {
		t.Fatal("failed isolation falsely acknowledged")
	}
	mustMQ(t, f.db.Ping())
}
