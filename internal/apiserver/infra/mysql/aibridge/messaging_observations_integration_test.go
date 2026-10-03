//go:build integration

package aibridge

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	durable "github.com/FangcunMount/reliable-messaging/delivery/mysql"
	"github.com/google/uuid"
)

func observationCount(t *testing.T, db *sql.DB, kind string) uint64 {
	t.Helper()
	var count uint64
	mustMQ(t, db.QueryRow("SELECT recorded_count FROM ai_messaging_observations WHERE kind=?", kind).Scan(&count))
	return count
}

func TestMQDuplicateObservationSharesAckTransactionAndOriginalProjection(t *testing.T) {
	f := newMQFixture(t)
	ctx := context.Background()
	legacy := &Store{DB: f.db}
	mustMQ(t, legacy.StageStart(ctx, f.request))
	state := &pb.StateEvent{EventId: uuid.NewString(), RequestId: f.request.RequestID, SessionId: f.session, Actor: &pb.Actor{OrgId: f.scope.OrganizationID, SubjectId: f.scope.SubjectID}, TesteeId: f.request.TesteeID, Version: 8, Status: "cancelled"}
	event := f.protect(pb.MessagingKind_INTERPRETATION_STATE, state.EventId, f.request.RequestID, "", &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: state}}, true)
	seals := 0
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return f.receive(tx, event, &seals) }))
	var ackID, hash string
	var firstWire []byte
	var created time.Time
	mustMQ(t, f.db.QueryRow("SELECT ack_id FROM ai_messaging_inbox WHERE message_id=?", state.EventId).Scan(&ackID))
	mustMQ(t, f.db.QueryRow("SELECT body_sha256,wire,created_at FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&hash, &firstWire, &created))
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		return f.store.Outbox.Published(ctx, tx, durable.Identity{Producer: "qs-server", Destination: "qs-ai", MessageID: ackID}, hash, 30)
	}))
	baseline := observationCount(t, f.db, "duplicate_event")
	tx, err := f.db.BeginTx(ctx, nil)
	mustMQ(t, err)
	mustMQ(t, f.receive(tx, event, &seals))
	mustMQ(t, tx.Rollback())
	if observationCount(t, f.db, "duplicate_event") != baseline {
		t.Fatal("duplicate observation escaped original rollback")
	}
	var stage string
	mustMQ(t, f.db.QueryRow("SELECT stage FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&stage))
	if stage != "confirmed" {
		t.Fatal("ACK rearm escaped rollback", stage)
	}
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return f.receive(tx, event, &seals) }))
	if observationCount(t, f.db, "duplicate_event") != baseline+1 {
		t.Fatal("committed duplicate missing")
	}
	var wire []byte
	var finalTime time.Time
	mustMQ(t, f.db.QueryRow("SELECT wire,created_at,stage FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&wire, &finalTime, &stage))
	projection, err := legacy.Projection(ctx, f.request.RequestID)
	mustMQ(t, err)
	if seals != 1 || !bytes.Equal(wire, firstWire) || !created.Equal(finalTime) || stage != "staged" || projection.Version != 8 || projection.Status != "cancelled" {
		t.Fatal("duplicate reconstructed original ACK/projection")
	}
	state.Version++
	conflict := f.protect(pb.MessagingKind_INTERPRETATION_STATE, state.EventId, f.request.RequestID, "", &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: state}}, true)
	err = f.tx(func(tx *sql.Tx) error { return f.receive(tx, conflict, &seals) })
	if !errors.Is(err, app.ErrConflict) || observationCount(t, f.db, "duplicate_event") != baseline+1 {
		t.Fatal("identity conflict counted as trusted duplicate", err)
	}
	mustMQ(t, f.db.Ping())
}

func TestMQPayloadObservationCommitsOnlyTechnicalFactAndPreservesReadTransaction(t *testing.T) {
	f := newMQFixture(t)
	ctx := context.Background()
	kind := "payload_serve_reference_mismatch"
	baseline := observationCount(t, f.db, kind)
	tx, err := f.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	mustMQ(t, err)
	reader := &MessagingReader{DB: f.db, Store: f.store}
	mustMQ(t, reader.ObserveMessagePayloadFailure(ctx, kind))
	var one int
	mustMQ(t, tx.QueryRow("SELECT 1").Scan(&one))
	mustMQ(t, tx.Rollback())
	if observationCount(t, f.db, kind) != baseline+1 {
		t.Fatal("technical fact lost with read rollback")
	}
	var identities int
	mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_messaging_outbox WHERE aggregate_key=?", f.request.RequestID).Scan(&identities))
	if identities != 0 {
		t.Fatal("payload error created business message")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if reader.ObserveMessagePayloadFailure(cancelled, kind) == nil || observationCount(t, f.db, kind) != baseline+1 {
		t.Fatal("cancelled observation committed")
	}
	if reader.ObserveMessagePayloadFailure(ctx, "untrusted-body") == nil || reader.ObserveMessagePayloadFailure(ctx, "duplicate_event") == nil {
		t.Fatal("untrusted category or duplicate bypass accepted")
	}
	mustMQ(t, f.db.Ping())
}

func TestMQTechnicalRecordingCoverageAndSchemaFailureAreExplicit(t *testing.T) {
	f := newMQFixture(t)
	values, err := MessagingSnapshot(t.Context(), f.db)
	mustMQ(t, err)
	if values["duplicate_observations_available"] != 1 || values["payload_error_observations_available"] != 1 || values["observations_history_complete"] != 0 || values["observations_recording_since_epoch_seconds"] <= 0 {
		t.Fatal("recorded facts misrepresented as complete history", values)
	}
	for _, kind := range observationKinds {
		if _, ok := values["recorded_"+kind]; !ok {
			t.Fatal("seeded kind unavailable", kind)
		}
	}
	tx, err := f.db.BeginTx(t.Context(), nil)
	mustMQ(t, err)
	defer tx.Rollback()
	// Only this uncommitted test change is visible to startup validation.
	_, err = tx.Exec("DELETE FROM ai_messaging_observations WHERE kind='duplicate_event'")
	mustMQ(t, err)
	if err = RequireMessagingObservations(t.Context(), tx); err == nil || strings.Contains(err.Error(), "SELECT") {
		t.Fatal("partial coverage allowed startup or leaked storage", err)
	}
	missing, err := collectTechnicalObservations(t.Context(), tx)
	mustMQ(t, err)
	if len(missing) != 2 || missing["duplicate_observations_available"] != 0 || missing["payload_error_observations_available"] != 0 {
		t.Fatal("partial ledger fabricated zero counts", missing)
	}
	mustMQ(t, tx.Rollback())
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return RequireMessagingObservations(t.Context(), tx) }))
	mustMQ(t, f.db.Ping())
}
