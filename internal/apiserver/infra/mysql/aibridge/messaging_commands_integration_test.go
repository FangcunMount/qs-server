//go:build integration

package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/google/uuid"
)

func (f *mqFixture) commandStore(calls *int) *MessagingCommandStore {
	return &MessagingCommandStore{Store: &Store{DB: f.db}, Messaging: f.store, Seal: func(kind pb.MessagingKind, id, aggregate, org, stamp string, body *pb.MessagingBody) (*app.PreparedMessaging, error) {
		*calls++
		return app.ProtectMessaging(kind, id, aggregate, "", org, stamp, body, f.qsSign, f.aiCrypt.Public())
	}}
}

func TestMQSubmissionSharesOriginalRequestAndAvoidsLegacyDelivery(t *testing.T) {
	f := newMQFixture(t)
	calls := 0
	store := f.commandStore(&calls)
	service := &app.Service{Store: store}
	mustMQ(t, service.Start(context.Background(), f.request))
	var originalWire []byte
	mustMQ(t, f.db.QueryRow("SELECT wire FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&originalWire))
	mustMQ(t, service.Start(context.Background(), f.request))
	var wire []byte
	mustMQ(t, f.db.QueryRow("SELECT wire FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&wire))
	if string(wire) != string(originalWire) || calls != 1 {
		t.Fatal("duplicate resealed submitted intent")
	}
	old, err := store.Original(context.Background(), f.request.RequestID)
	mustMQ(t, err)
	if old.Goal != f.request.Goal || old.Actor != f.request.Actor {
		t.Fatal("original request changed")
	}
	var count int
	mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_bridge_commands WHERE request_id=?", f.request.RequestID).Scan(&count))
	if count != 0 {
		t.Fatal("second gRPC delivery path staged")
	}
	if _, err := store.Pending(context.Background(), 20); !errors.Is(err, app.ErrManagementUnavailable) {
		t.Fatal("legacy scanner allowed", err)
	}
	changed := f.request
	changed.Goal = "same id different intent"
	if err := service.Start(context.Background(), changed); !errors.Is(err, app.ErrConflict) {
		t.Fatal("identity conflict accepted", err)
	}
	// Start's decision precedes Change publishing, independent of model completion.
	_, err = f.db.Exec("UPDATE ai_bridge_requests SET session_id=? WHERE request_id=?", f.session, f.request.RequestID)
	mustMQ(t, err)
	change := app.Change{CommandID: uuid.NewString(), SessionID: f.session, Actor: f.request.Actor, Action: "cancel", ExpectedVersion: 2}
	mustMQ(t, service.Change(context.Background(), f.request.RequestID, change))
	mustMQ(t, service.Change(context.Background(), f.request.RequestID, change))
	if calls != 2 {
		t.Fatal("duplicate change resealed")
	}
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		pending, err := f.store.Outbox.Pending(context.Background(), tx, 20)
		if err == nil && (len(pending) != 1 || pending[0].MessageID != f.request.RequestID) {
			t.Fatal("next aggregate command overtook decision")
		}
		return err
	}))
	change.Actor.SubjectID = "different actor"
	if err := service.Change(context.Background(), f.request.RequestID, change); !errors.Is(err, app.ErrConflict) {
		t.Fatal("actor rebinding accepted", err)
	}
}

func TestMQSubmissionSealFailureRollsBackRequestAndOperation(t *testing.T) {
	f := newMQFixture(t)
	calls := 0
	store := f.commandStore(&calls)
	sentinel := errors.New("local sealing failed")
	store.Seal = func(pb.MessagingKind, string, string, string, string, *pb.MessagingBody) (*app.PreparedMessaging, error) {
		return nil, sentinel
	}
	if err := (&app.Service{Store: store}).Start(context.Background(), f.request); !errors.Is(err, sentinel) {
		t.Fatal("failure hidden", err)
	}
	for _, table := range []string{"ai_bridge_requests", "ai_bridge_request_assessments"} {
		var count int
		mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE request_id=?", f.request.RequestID).Scan(&count))
		if count != 0 {
			t.Fatal("orphan request", table)
		}
	}
	for _, table := range []string{"ai_messaging_operations", "ai_messaging_outbox", "ai_messaging_aggregates"} {
		var count int
		mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE aggregate_key=?", f.request.RequestID).Scan(&count))
		if count != 0 {
			t.Fatal("orphan message", table)
		}
	}
}
