//go:build integration

package aibridge

import (
	"bytes"
	"database/sql"
	"errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"testing"
	"time"
)

func handoffFixture(f *mqFixture, calls *int) *MessagingLegacyHandoff {
	return &MessagingLegacyHandoff{Store: f.store, Seal: func(k pb.MessagingKind, id, agg, org, at string, b *pb.MessagingBody) (*app.PreparedMessaging, error) {
		*calls++
		return app.ProtectMessaging(k, id, agg, "", org, at, b, f.qsSign, f.aiCrypt.Public())
	}}
}
func TestMQLegacyHandoffAtomicIdentityBudgetAndOriginalTime(t *testing.T) {
	f := newMQFixture(t)
	mustMQ(t, seedLegacyStartFixture(t.Context(), f.request, &Store{DB: f.db}))
	calls := 0
	h := handoffFixture(f, &calls)
	_, e := f.db.Exec("UPDATE ai_bridge_requests SET created_at='2026-10-03 01:02:03.123456' WHERE request_id=?", f.request.RequestID)
	mustMQ(t, e)
	_, e = f.db.Exec("UPDATE ai_bridge_commands SET attempts=3,available_at='2026-10-03 02:00:00.123456' WHERE command_id=?", f.request.RequestID)
	mustMQ(t, e)
	tx, e := f.db.BeginTx(t.Context(), nil)
	mustMQ(t, e)
	moved, e := h.StageSingle(t.Context(), tx, f.request.RequestID)
	mustMQ(t, e)
	if !moved {
		t.Fatal("pending source not moved")
	}
	mustMQ(t, tx.Rollback())
	for table, column := range map[string]string{"ai_messaging_outbox": "message_id", "ai_messaging_operations": "command_id", "ai_messaging_legacy_commands": "command_id"} {
		var count int
		mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+column+"=?", f.request.RequestID).Scan(&count))
		if count != 0 {
			t.Fatal("handoff escaped caller rollback")
		}
	}
	calls = 0
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		moved, e := h.StageSingle(t.Context(), tx, f.request.RequestID)
		if !moved {
			t.Fatal("no transfer")
		}
		return e
	}))
	var wire, original []byte
	var attempts int
	var sourceHash, auditHash, at string
	var delivered bool
	mustMQ(t, f.db.QueryRow("SELECT payload,payload_hash,attempts,delivered FROM ai_bridge_commands WHERE command_id=?", f.request.RequestID).Scan(&original, &sourceHash, &attempts, &delivered))
	if delivered || attempts != 3 {
		t.Fatal("ownership became delivered or source budget reset")
	}
	mustMQ(t, f.db.QueryRow("SELECT wire,attempts FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&wire, &attempts))
	if attempts != 3 {
		t.Fatal("transport migration reset budget")
	}
	mustMQ(t, f.db.QueryRow("SELECT source_payload_hash,source_original_time FROM ai_messaging_legacy_commands WHERE command_id=?", f.request.RequestID).Scan(&auditHash, &at))
	if sourceHash != auditHash || at != "2026-10-03T09:02:03.123456+08:00" {
		t.Fatal("original hash or time changed")
	}
	e1, e := app.AuthenticateMessaging(wire, app.CommandsTopic, protected.Keyring{Decrypt: map[string]jose.JSONWebKey{f.aiCrypt.KeyID: f.aiCrypt}, Signers: map[string]protected.TrustedSigner{f.qsSign.KeyID: {Producer: "qs-server", Key: f.qsSign.Public()}}})
	mustMQ(t, e)
	body, e := app.ParseMessagingBody(e1, e1.GetInlineBody())
	mustMQ(t, e)
	if e1.MessageId != f.request.RequestID || e1.OriginalOccurredAt != at || body.GetStart().Goal != f.request.Goal {
		t.Fatal("handoff rebuilt business identity")
	}
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		moved, e := h.StageSingle(t.Context(), tx, f.request.RequestID)
		if moved {
			t.Fatal("duplicate migration staged again")
		}
		return e
	}))
	var again []byte
	mustMQ(t, f.db.QueryRow("SELECT wire FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&again))
	if calls != 1 || !bytes.Equal(wire, again) {
		t.Fatal("duplicate migration resealed wire")
	}
	if e := f.db.Ping(); e != nil {
		t.Fatal("handoff closed borrowed pool")
	}
}
func TestMQLegacyHandoffPreservesDeliveredAndExhaustedHistory(t *testing.T) {
	for _, scenario := range []string{"delivered", "exhausted"} {
		t.Run(scenario, func(t *testing.T) {
			f := newMQFixture(t)
			mustMQ(t, seedLegacyStartFixture(t.Context(), f.request, &Store{DB: f.db}))
			calls := 0
			h := handoffFixture(f, &calls)
			if scenario == "delivered" {
				_, e := f.db.Exec("UPDATE ai_bridge_commands SET delivered=TRUE WHERE command_id=?", f.request.RequestID)
				mustMQ(t, e)
			} else {
				_, e := f.db.Exec("UPDATE ai_bridge_commands SET attempts=19 WHERE command_id=?", f.request.RequestID)
				mustMQ(t, e)
			}
			mustMQ(t, f.tx(func(tx *sql.Tx) error {
				moved, e := h.StageSingle(t.Context(), tx, f.request.RequestID)
				if moved != (scenario == "exhausted") {
					t.Fatal("wrong historical transfer")
				}
				return e
			}))
			if scenario == "delivered" {
				if calls != 0 {
					t.Fatal("delivered record resealed")
				}
				return
			}
			var stage, code string
			var attempts int
			mustMQ(t, f.db.QueryRow("SELECT stage,error_code,attempts FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&stage, &code, &attempts))
			if stage != "held" || code != "legacy_delivery_budget_exhausted" || attempts != 8 {
				t.Fatal("historical budget got new retry allowance")
			}
		})
	}
}
func TestMQLegacyHandoffRefusesInventedOrderAndPreservesUnknownChangeTime(t *testing.T) {
	f := newMQFixture(t)
	legacy := &Store{DB: f.db}
	mustMQ(t, seedLegacyStartFixture(t.Context(), f.request, legacy))
	_, e := f.db.Exec("UPDATE ai_bridge_requests SET session_id=? WHERE request_id=?", f.session, f.request.RequestID)
	mustMQ(t, e)
	c := app.Change{CommandID: uuid.NewString(), SessionID: f.session, Actor: f.request.Actor, Action: "cancel", ExpectedVersion: 1}
	mustMQ(t, seedLegacyChangeFixture(t.Context(), f.request.RequestID, c, legacy))
	calls := 0
	h := handoffFixture(f, &calls)
	e = f.tx(func(tx *sql.Tx) error { _, e := h.StageSingle(t.Context(), tx, c.CommandID); return e })
	if !errors.Is(e, ErrLegacyCommandOrderUnknown) || calls != 0 {
		t.Fatal("invented historical commit order", e)
	}
	// Once original Start has a real historical delivery, exactly one Change is
	// unowned; its retry timestamp still cannot become original occurrence time.
	_, e = f.db.Exec("UPDATE ai_bridge_commands SET delivered=TRUE WHERE command_id=?", f.request.RequestID)
	mustMQ(t, e)
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		moved, e := h.StageSingle(t.Context(), tx, c.CommandID)
		mustMQ(t, e)
		if !moved {
			t.Fatal("single pending change not transferred")
		}
		return e
	}))
	var at string
	mustMQ(t, f.db.QueryRow("SELECT source_original_time FROM ai_messaging_legacy_commands WHERE command_id=?", c.CommandID).Scan(&at))
	if at != "" {
		t.Fatal("retry time replaced unknown original time")
	}
	var sourceAvailable time.Time
	mustMQ(t, f.db.QueryRow("SELECT available_at FROM ai_bridge_commands WHERE command_id=?", c.CommandID).Scan(&sourceAvailable))
	if sourceAvailable.IsZero() {
		t.Fatal("historical retry evidence lost")
	}
}

func TestMQLegacyHandoffValidatesFirstSourceBeforeNullableIndexBackfill(t *testing.T) {
	for _, scenario := range []string{"missing_indexes", "conflicting_index", "source_tamper"} {
		t.Run(scenario, func(t *testing.T) {
			f := newMQFixture(t)
			mustMQ(t, seedLegacyStartFixture(t.Context(), f.request, &Store{DB: f.db}))
			calls := 0
			h := handoffFixture(f, &calls)
			_, e := f.db.Exec("UPDATE ai_bridge_requests SET organization_id=NULL,subject_id=NULL,testee_id=NULL,created_at=NULL WHERE request_id=?", f.request.RequestID)
			mustMQ(t, e)
			if scenario == "conflicting_index" {
				_, e = f.db.Exec("UPDATE ai_bridge_requests SET organization_id=1 WHERE request_id=?", f.request.RequestID)
				mustMQ(t, e)
			}
			if scenario == "source_tamper" {
				_, e = f.db.Exec("UPDATE ai_bridge_commands SET payload=JSON_SET(payload,'$.goal','modified') WHERE command_id=?", f.request.RequestID)
				mustMQ(t, e)
			}
			e = f.tx(func(tx *sql.Tx) error { _, e := h.StageSingle(t.Context(), tx, f.request.RequestID); return e })
			if scenario != "missing_indexes" {
				if !errors.Is(e, app.ErrConflict) || calls != 0 {
					t.Fatal("untrusted source/index staged", e)
				}
				return
			}
			mustMQ(t, e)
			var org, testee, subject string
			var created sql.NullTime
			mustMQ(t, f.db.QueryRow("SELECT CAST(organization_id AS CHAR),subject_id,CAST(testee_id AS CHAR),created_at FROM ai_bridge_requests WHERE request_id=?", f.request.RequestID).Scan(&org, &subject, &testee, &created))
			if org != f.request.Actor.OrgID || subject != f.request.Actor.SubjectID || testee != f.request.TesteeID || created.Valid {
				t.Fatal("index hydration changed identity or invented time")
			}
			var at string
			mustMQ(t, f.db.QueryRow("SELECT source_original_time FROM ai_messaging_legacy_commands WHERE command_id=?", f.request.RequestID).Scan(&at))
			if at != "" {
				t.Fatal("unknown creation time replaced")
			}
		})
	}
}
