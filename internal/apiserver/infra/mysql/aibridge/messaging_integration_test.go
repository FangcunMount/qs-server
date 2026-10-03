//go:build integration

package aibridge

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	durable "github.com/FangcunMount/reliable-messaging/delivery/mysql"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
)

type mqFixture struct {
	t                                *testing.T
	db                               *sql.DB
	store                            *MessagingStore
	request                          app.Start
	session, run                     string
	scope                            app.OperationScope
	qsSign, qsCrypt, aiSign, aiCrypt jose.JSONWebKey
}

func newMQFixture(t *testing.T) *mqFixture {
	t.Helper()
	dsn := os.Getenv("QS_AI_MQ_TEST_DSN")
	if dsn == "" {
		t.Fatal("required disposable QS MQ MySQL DSN is missing")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		t.Fatal(err)
	}
	f := &mqFixture{t: t, db: db, store: NewMessagingStore(), session: uuid.NewString(), run: uuid.NewString()}
	f.request = app.Start{RequestID: uuid.NewString(), Actor: app.Actor{OrgID: "18446744073709551615", SubjectID: "42"}, TesteeID: "7", AssessmentIDs: []string{"9"}, Goal: "真实原事务：中文"}
	f.scope = app.OperationScope{OrganizationID: f.request.Actor.OrgID, SubjectID: "42", ResourceID: f.request.RequestID}
	key := func(id string) jose.JSONWebKey {
		k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		return jose.JSONWebKey{Key: k, KeyID: id}
	}
	f.qsSign, f.qsCrypt, f.aiSign, f.aiCrypt = key("qs-sign"), key("qs-crypt"), key("ai-sign"), key("ai-crypt")
	t.Cleanup(func() {
		for _, aggregate := range []string{f.request.RequestID, f.run} {
			for _, table := range []string{"ai_messaging_inbox", "ai_messaging_operations", "ai_messaging_outbox", "ai_messaging_aggregates", "ai_messaging_failures"} {
				if _, err := db.Exec("DELETE FROM "+table+" WHERE aggregate_key=?", aggregate); err != nil {
					t.Error(err)
				}
			}
		}
		if _, err := db.Exec("DELETE FROM ai_messaging_evaluation_states WHERE run_id=?", f.run); err != nil {
			t.Error(err)
		}
		for _, table := range []string{"ai_bridge_events", "ai_bridge_commands", "ai_bridge_requests"} {
			if _, err := db.Exec("DELETE FROM "+table+" WHERE request_id=?", f.request.RequestID); err != nil {
				t.Error(err)
			}
		}
		_ = db.Close()
	})
	return f
}

func (f *mqFixture) tx(fn func(*sql.Tx) error) error {
	tx, err := f.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
func (f *mqFixture) startBody() *pb.MessagingBody {
	return &pb.MessagingBody{Value: &pb.MessagingBody_Start{Start: &pb.StartCommand{RequestId: f.request.RequestID, Actor: &pb.Actor{OrgId: f.scope.OrganizationID, SubjectId: f.scope.SubjectID}, TesteeId: f.request.TesteeID, AssessmentIds: f.request.AssessmentIDs, Goal: f.request.Goal}}}
}
func (f *mqFixture) protect(kind pb.MessagingKind, id, aggregate, correlation string, body *pb.MessagingBody, fromAI bool) *app.PreparedMessaging {
	f.t.Helper()
	signing, recipient := f.qsSign, f.aiCrypt.Public()
	if fromAI {
		signing, recipient = f.aiSign, f.qsCrypt.Public()
	}
	m, err := app.ProtectMessaging(kind, id, aggregate, correlation, f.scope.OrganizationID, "2026-10-03T16:00:00.123456+08:00", body, signing, recipient)
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}
func (f *mqFixture) stage(tx *sql.Tx, kind pb.MessagingKind, id, agg string, body *pb.MessagingBody, scope app.OperationScope, calls *int) (uint64, error) {
	e, raw, err := app.PrepareMessaging(kind, id, agg, "", body)
	if err != nil {
		return 0, err
	}
	e.OriginalOccurredAt = "2026-10-03T16:00:00.123456+08:00"
	return f.store.StageOperation(context.Background(), tx, e, raw, scope, func() (*app.PreparedMessaging, error) {
		if calls != nil {
			*calls++
		}
		return f.protect(kind, id, agg, "", body, false), nil
	})
}
func (f *mqFixture) receive(tx *sql.Tx, m *app.PreparedMessaging, calls *int) error {
	e, err := app.AuthenticateMessaging(m.Wire, app.EventsTopic, protected.Keyring{Decrypt: map[string]jose.JSONWebKey{f.qsCrypt.KeyID: f.qsCrypt}, Signers: map[string]protected.TrustedSigner{f.aiSign.KeyID: {Producer: "qs-ai", Key: f.aiSign.Public()}}})
	if err != nil {
		return err
	}
	return f.store.ReceiveEvent(context.Background(), tx, e, m.Body, m.Wire, func(id, org string) (*app.PreparedMessaging, error) {
		if calls != nil {
			*calls++
		}
		return app.ProtectMessaging(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT, id, e.AggregateKey, "", org, "", &pb.MessagingBody{Value: &pb.MessagingBody_EventAcknowledgement{EventAcknowledgement: &pb.MessagingEventAcknowledgement{EventId: e.MessageId, EventBodySha256: e.BodySha256, EventKind: e.Kind, Outcome: pb.MessagingEventAcknowledgement_STORED}}}, f.qsSign, f.aiCrypt.Public())
	})
}
func mustMQ(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestMQOperationOriginalTransactionIdentityAndSingleSeal(t *testing.T) {
	f := newMQFixture(t)
	ctx := context.Background()
	body := f.startBody()
	calls := 0
	e, raw, err := app.PrepareMessaging(pb.MessagingKind_START, f.request.RequestID, f.request.RequestID, "", body)
	mustMQ(t, err)
	if _, err = f.store.StageOperation(ctx, nil, e, raw, f.scope, func() (*app.PreparedMessaging, error) { calls++; return nil, nil }); err == nil {
		t.Fatal("nil transaction accepted")
	}
	tx, err := f.db.BeginTx(ctx, nil)
	mustMQ(t, err)
	_, err = f.stage(tx, pb.MessagingKind_START, f.request.RequestID, f.request.RequestID, body, f.scope, &calls)
	mustMQ(t, err)
	mustMQ(t, tx.Rollback())
	var count int
	mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_messaging_operations WHERE command_id=?", f.request.RequestID).Scan(&count))
	if count != 0 {
		t.Fatal("operation escaped rollback")
	}
	calls = 0
	for i := 0; i < 2; i++ {
		mustMQ(t, f.tx(func(tx *sql.Tx) error {
			seq, err := f.stage(tx, pb.MessagingKind_START, f.request.RequestID, f.request.RequestID, body, f.scope, &calls)
			if seq != 1 {
				t.Errorf("sequence=%d", seq)
			}
			return err
		}))
	}
	if calls != 1 {
		t.Fatalf("duplicate resealed %d times", calls)
	}
	bad := f.startBody()
	bad.GetStart().Goal = "changed"
	err = f.tx(func(tx *sql.Tx) error {
		_, err := f.stage(tx, pb.MessagingKind_START, f.request.RequestID, f.request.RequestID, bad, f.scope, &calls)
		return err
	})
	if !errors.Is(err, app.ErrConflict) {
		t.Fatalf("changed identity: %v", err)
	}
	err = f.tx(func(tx *sql.Tx) error {
		op, err := f.store.Operation(ctx, tx, f.scope, f.request.RequestID)
		if op.Status != "submitted" || op.Decision != "" || op.TransportStatus != "staged" {
			t.Errorf("unexpected operation: %+v", op)
		}
		return err
	})
	mustMQ(t, err)
	for _, org := range []string{"1", "18446744073709551615evil"} {
		badScope := f.scope
		badScope.OrganizationID = org
		err = f.tx(func(tx *sql.Tx) error {
			_, err := f.store.Operation(ctx, tx, badScope, f.request.RequestID)
			return err
		})
		if err == nil {
			t.Fatal("wrong/coerced org could read")
		}
	}
	badScope := f.scope
	badScope.SubjectID = "another"
	err = f.tx(func(tx *sql.Tx) error {
		_, err := f.store.Operation(ctx, tx, badScope, f.request.RequestID)
		return err
	})
	if !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("wrong owner: %v", err)
	}
	mustMQ(t, f.db.Ping()) // borrowed pool remains usable
}

func TestMQPendingRequiresDurableDecisionAndHeldBlocksNext(t *testing.T) {
	f := newMQFixture(t)
	ctx := context.Background()
	scope := f.scope
	scope.OrganizationID = "1"
	scope.ResourceID = f.run
	body := &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationStart{EvaluationStart: &pb.EvaluationStartCommand{Scope: &pb.EvaluationQuery{RunId: f.run, OrganizationId: 1, OperatorUserId: 42}, ExpectedVersion: 1, Confirm: true}}}
	ids := []string{uuid.NewString(), uuid.NewString()}
	for _, id := range ids {
		mustMQ(t, f.tx(func(tx *sql.Tx) error {
			_, err := f.stage(tx, pb.MessagingKind_EVALUATION_START, id, f.run, body, scope, nil)
			return err
		}))
	}
	var hash string
	mustMQ(t, f.db.QueryRow("SELECT body_sha256 FROM ai_messaging_outbox WHERE message_id=?", ids[0]).Scan(&hash))
	id := durable.Identity{Producer: "qs-server", Destination: "qs-ai", MessageID: ids[0]}
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		p, err := f.store.Outbox.Pending(ctx, tx, 20)
		if len(p) != 1 || p[0].MessageID != ids[0] {
			t.Errorf("out of order: %+v", p)
		}
		if err != nil {
			return err
		}
		return f.store.Outbox.Published(ctx, tx, id, hash, 30)
	}))
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		op, err := f.store.Operation(ctx, tx, scope, ids[0])
		if op.Status != "submitted" || op.TransportStatus != "awaiting_receipt" {
			t.Errorf("PUB was treated as decision: %+v", op)
		}
		return err
	}))
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return f.store.Outbox.Hold(ctx, tx, id, hash, "technical_hold") }))
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		p, err := f.store.Outbox.Pending(ctx, tx, 20)
		if len(p) != 0 {
			t.Errorf("held predecessor was bypassed: %+v", p)
		}
		return err
	}))
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return f.store.Outbox.Confirm(ctx, tx, id, hash) }))
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		p, err := f.store.Outbox.Pending(ctx, tx, 20)
		if len(p) != 1 || p[0].MessageID != ids[1] {
			t.Errorf("next command missing: %+v", p)
		}
		return err
	}))
}

func TestMQReceiptLostAckDuplicateAndLateReceiptDoNotRegress(t *testing.T) {
	f := newMQFixture(t)
	ctx := context.Background()
	legacy := &Store{DB: f.db}
	mustMQ(t, legacy.StageStart(ctx, f.request))
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		_, err := f.stage(tx, pb.MessagingKind_START, f.request.RequestID, f.request.RequestID, f.startBody(), f.scope, nil)
		return err
	}))
	state := &pb.StateEvent{EventId: uuid.NewString(), RequestId: f.request.RequestID, SessionId: f.session, Actor: &pb.Actor{OrgId: f.scope.OrganizationID, SubjectId: f.scope.SubjectID}, TesteeId: f.request.TesteeID, Version: 8, Status: "cancelled"}
	event := f.protect(pb.MessagingKind_INTERPRETATION_STATE, state.EventId, f.request.RequestID, "", &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: state}}, true)
	calls := 0
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return f.receive(tx, event, &calls) }))
	var hash string
	mustMQ(t, f.db.QueryRow("SELECT body_sha256 FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&hash))
	receipt := f.protect(pb.MessagingKind_COMMAND_RECEIPT, uuid.NewString(), f.request.RequestID, f.request.RequestID, &pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: &pb.MessagingCommandReceipt{CommandId: f.request.RequestID, CommandBodySha256: hash, Decision: pb.MessagingDecision_ACCEPTED, OriginalReceipt: &pb.MessagingCommandReceipt_WorkflowReceipt{WorkflowReceipt: &pb.Receipt{SessionId: f.session, RunId: uuid.NewString(), Version: 2, Status: "queued"}}}}}, true)
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return f.receive(tx, receipt, &calls) }))
	got, err := legacy.Projection(ctx, f.request.RequestID)
	mustMQ(t, err)
	if got.Version != 8 || got.Status != "cancelled" {
		t.Fatalf("late receipt regressed projection: %+v", got)
	}
	var ackID, ackHash string
	var firstWire []byte
	mustMQ(t, f.db.QueryRow("SELECT ack_id FROM ai_messaging_inbox WHERE message_id=?", receipt.Envelope.MessageId).Scan(&ackID))
	mustMQ(t, f.db.QueryRow("SELECT body_sha256,wire FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&ackHash, &firstWire))
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		return f.store.Outbox.Published(ctx, tx, durable.Identity{Producer: "qs-server", Destination: "qs-ai", MessageID: ackID}, ackHash, 30)
	}))
	// Simulated final ACK loss and receiver reopen: exact ACK is rearmed, not resealed.
	reopened := NewMessagingStore()
	f.store = reopened
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return f.receive(tx, receipt, &calls) }))
	var lastWire []byte
	var stage string
	mustMQ(t, f.db.QueryRow("SELECT wire,stage FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&lastWire, &stage))
	if calls != 2 || !bytes.Equal(firstWire, lastWire) || stage != "staged" {
		t.Fatalf("duplicate changed ack: seals=%d stage=%s", calls, stage)
	}
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		op, err := f.store.Operation(ctx, tx, f.scope, f.request.RequestID)
		if op.Status != "accepted" || op.TransportStatus != "confirmed" || !strings.Contains(string(op.Receipt), "queued") {
			t.Errorf("receipt missing: %+v", op)
		}
		return err
	}))
}

func TestMQProjectionInboxAndAckRollbackTogether(t *testing.T) {
	f := newMQFixture(t)
	legacy := &Store{DB: f.db}
	mustMQ(t, legacy.StageStart(context.Background(), f.request))
	state := &pb.StateEvent{EventId: uuid.NewString(), RequestId: f.request.RequestID, SessionId: f.session, Actor: &pb.Actor{OrgId: f.scope.OrganizationID, SubjectId: f.scope.SubjectID}, TesteeId: f.request.TesteeID, Version: 1, Status: "running"}
	event := f.protect(pb.MessagingKind_INTERPRETATION_STATE, state.EventId, f.request.RequestID, "", &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: state}}, true)
	injected := errors.New("ack persistence failure")
	err := f.tx(func(tx *sql.Tx) error {
		return f.store.ReceiveEvent(context.Background(), tx, event.Envelope, event.Body, event.Wire, func(string, string) (*app.PreparedMessaging, error) { return nil, injected })
	})
	if !errors.Is(err, injected) {
		t.Fatalf("failure hidden: %v", err)
	}
	for _, table := range []string{"ai_messaging_inbox", "ai_messaging_outbox"} {
		var count int
		mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE aggregate_key=?", f.request.RequestID).Scan(&count))
		if count != 0 {
			t.Fatalf("orphan in %s", table)
		}
	}
	got, err := legacy.Projection(context.Background(), f.request.RequestID)
	mustMQ(t, err)
	if got != nil {
		t.Fatal("business projection escaped rollback")
	}
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return f.receive(tx, event, nil) }))
}

func TestMQReceiptMismatchNeverConfirmsCommand(t *testing.T) {
	f := newMQFixture(t)
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		_, err := f.stage(tx, pb.MessagingKind_START, f.request.RequestID, f.request.RequestID, f.startBody(), f.scope, nil)
		return err
	}))
	event := f.protect(pb.MessagingKind_COMMAND_RECEIPT, uuid.NewString(), f.request.RequestID, f.request.RequestID, &pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: &pb.MessagingCommandReceipt{CommandId: f.request.RequestID, CommandBodySha256: strings.Repeat("a", 64), Decision: pb.MessagingDecision_REJECTED, Code: "capacity"}}}, true)
	err := f.tx(func(tx *sql.Tx) error { return f.receive(tx, event, nil) })
	if !errors.Is(err, app.ErrConflict) {
		t.Fatalf("wrong hash accepted: %v", err)
	}
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		op, err := f.store.Operation(context.Background(), tx, f.scope, f.request.RequestID)
		if op.Status != "submitted" || op.TransportStatus != "staged" {
			t.Errorf("bad receipt settled: %+v", op)
		}
		return err
	}))
}

func TestMQEvaluationEventsDeduplicateBeforeMonotonicProjection(t *testing.T) {
	f := newMQFixture(t)
	ctx := context.Background()
	makeEvent := func(sequence uint64, version int64, status string) *app.PreparedMessaging {
		return f.protect(pb.MessagingKind_EVALUATION_STATE, uuid.NewString(), f.run, "", &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationState{EvaluationState: &pb.EvaluationRuntimeState{RunId: f.run, OrganizationId: f.scope.OrganizationID, EventSequence: sequence, Version: version, Status: status}}}, true)
	}
	newer := makeEvent(3, 5, "completed")
	older := makeEvent(1, 2, "running")
	for _, event := range []*app.PreparedMessaging{newer, older, newer} {
		mustMQ(t, f.tx(func(tx *sql.Tx) error { return f.receive(tx, event, nil) }))
	}
	var sequence uint64
	var version int64
	mustMQ(t, f.db.QueryRow("SELECT event_sequence,version FROM ai_messaging_evaluation_states WHERE run_id=?", f.run).Scan(&sequence, &version))
	if sequence != 3 || version != 5 {
		t.Fatal("evaluation projection regressed")
	}
	for _, event := range []*app.PreparedMessaging{makeEvent(3, 5, "different"), makeEvent(4, 4, "running")} {
		err := f.tx(func(tx *sql.Tx) error { return f.receive(tx, event, nil) })
		if !errors.Is(err, app.ErrConflict) {
			t.Fatalf("state conflict: %v", err)
		}
	}
	_ = ctx
}
