//go:build integration

package aibridge

import (
	"database/sql"
	"errors"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/google/uuid"
)

func TestMQRuntimeCommandStatisticsScopeHeldAndInheritedBudget(t *testing.T) {
	f := newMQFixture(t)
	f.request.Actor.OrgID = "1"
	f.scope.OrganizationID = "1"
	calls := 0
	s := f.commandStore(&calls)
	mustMQ(t, s.StageStart(t.Context(), f.request))
	_, err := f.db.Exec("UPDATE ai_bridge_requests SET session_id=? WHERE request_id=?", f.session, f.request.RequestID)
	mustMQ(t, err)
	change := app.Change{CommandID: uuid.NewString(), SessionID: f.session, Actor: f.request.Actor, Action: "cancel", ExpectedVersion: 1}
	mustMQ(t, s.StageChange(t.Context(), f.request.RequestID, change))
	retry := app.ParticipantRetry{CommandID: uuid.NewString(), ExpectedRunID: f.run, ExpectedVersion: 1, Reason: "reviewed", Confirm: true, ExpectedProviderInvocations: 1}
	mustMQ(t, s.SubmitParticipantRetry(t.Context(), app.DraftScope{OrganizationID: 1, OperatorUserID: 42}, f.session, retry))
	_, err = f.db.Exec("UPDATE ai_messaging_outbox SET attempts=5,stage='awaiting_receipt' WHERE message_id=?", f.request.RequestID)
	mustMQ(t, err)
	_, err = f.db.Exec("UPDATE ai_messaging_outbox SET attempts=8,stage='held' WHERE message_id=?", change.CommandID)
	mustMQ(t, err)
	_, err = f.db.Exec("UPDATE ai_messaging_outbox SET attempts=2 WHERE message_id=?", retry.CommandID)
	mustMQ(t, err)
	// Broker PUB has happened but no command business receipt has been persisted.
	got, err := s.GetRuntime(t.Context(), 1, f.request.RequestID)
	mustMQ(t, err)
	if got.CommandsPending != 3 || got.CommandAttempts != 15 {
		t.Fatal("held/inherited budget not counted", got)
	}
	// A final ACK and evaluation messages share the host table but are not request commands.
	ackBody := &pb.MessagingBody{Value: &pb.MessagingBody_EventAcknowledgement{EventAcknowledgement: &pb.MessagingEventAcknowledgement{EventId: uuid.NewString(), EventBodySha256: f.startBodyHash(t), EventKind: pb.MessagingKind_INTERPRETATION_STATE, Outcome: pb.MessagingEventAcknowledgement_STORED}}}
	ack := f.protect(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT, uuid.NewString(), f.request.RequestID, "", ackBody, false)
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return f.store.put(t.Context(), tx, ack, "1", 1, false, false) }))
	eval := app.EvaluationStart{CommandID: uuid.NewString(), ExpectedVersion: 1, Reason: "evaluation", Confirm: true}
	mustMQ(t, s.SubmitEvaluationStart(t.Context(), app.EvaluationScope{OrganizationID: 1, OperatorUserID: 42, RunID: f.run}, eval.CommandID, eval))
	foreign := newMQFixture(t)
	foreign.request.Actor.OrgID = "2"
	foreign.scope.OrganizationID = "2"
	otherCalls := 0
	mustMQ(t, foreign.commandStore(&otherCalls).StageStart(t.Context(), foreign.request))
	got, err = s.GetRuntime(t.Context(), 1, f.request.RequestID)
	mustMQ(t, err)
	if got.CommandsPending != 3 || got.CommandAttempts != 15 {
		t.Fatal("ACK/run/foreign counted", got)
	}
	backlog, err := s.RuntimeBacklog(t.Context(), 1)
	mustMQ(t, err)
	if backlog["pending_commands"] != 3 {
		t.Fatal("health scope differs", backlog)
	}
	assertNativeCurrentMessagingObservation(t, f, 3, 15)
	// Use the actual receiver decision path, including command hash and original session.
	var hash string
	mustMQ(t, f.db.QueryRow("SELECT body_sha256 FROM ai_messaging_operations WHERE command_id=?", f.request.RequestID).Scan(&hash))
	receipt := &pb.MessagingCommandReceipt{CommandId: f.request.RequestID, CommandBodySha256: hash, Decision: pb.MessagingDecision_ACCEPTED, OriginalReceipt: &pb.MessagingCommandReceipt_WorkflowReceipt{WorkflowReceipt: &pb.Receipt{SessionId: f.session, Version: 1, Status: "running"}}}
	body := &pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: receipt}}
	m := f.protect(pb.MessagingKind_COMMAND_RECEIPT, uuid.NewString(), f.request.RequestID, f.request.RequestID, body, true)
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		_, err := f.store.applyReceipt(t.Context(), tx, m.Envelope, receipt, m.Body)
		return err
	}))
	got, err = s.GetRuntime(t.Context(), 1, f.request.RequestID)
	mustMQ(t, err)
	if got.CommandsPending != 2 || got.CommandAttempts != 15 {
		t.Fatal("terminal attempts disappeared or accepted stayed pending", got)
	}
	assertNativeCurrentMessagingObservation(t, f, 2, 15)
}

func assertNativeCurrentMessagingObservation(t *testing.T, f *mqFixture, pending, attempts uint64) {
	t.Helper()
	tx, err := f.db.BeginTx(t.Context(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	mustMQ(t, err)
	defer func() { mustMQ(t, tx.Rollback()) }()
	v, err := ReadCurrentMessagingOrganization(t.Context(), tx, 1)
	mustMQ(t, err)
	if v.Requests == 0 || v.CommandsPending != pending || v.CommandAttempts != attempts {
		t.Fatal("borrowed native RO ledger read changed current Runtime scope/held/budget/terminal semantics", v)
	}
}

func (f *mqFixture) startBodyHash(t *testing.T) string {
	t.Helper()
	_, raw, err := app.PrepareMessaging(pb.MessagingKind_START, f.request.RequestID, f.request.RequestID, "", f.startBody())
	mustMQ(t, err)
	return messagingHash(raw)
}

func TestMQRuntimeIntegrityDoesNotHideOrphanConflictOrUnknownState(t *testing.T) {
	for _, scenario := range []string{"orphan_operation", "orphan_outbox", "scope", "both_foreign_scope", "foreign_operation_orphan", "foreign_outbox_orphan", "body_hash", "unknown_stage", "unknown_decision", "broker_confirmed_without_receipt"} {
		t.Run(scenario, func(t *testing.T) {
			f := newMQFixture(t)
			f.request.Actor.OrgID = "1"
			f.scope.OrganizationID = "1"
			calls := 0
			s := f.commandStore(&calls)
			mustMQ(t, s.StageStart(t.Context(), f.request))
			query := ""
			switch scenario {
			case "orphan_operation":
				query = "DELETE FROM ai_messaging_outbox WHERE message_id=?"
			case "orphan_outbox":
				query = "DELETE FROM ai_messaging_operations WHERE command_id=?"
			case "scope":
				query = "UPDATE ai_messaging_outbox SET organization_id=2 WHERE message_id=?"
			case "both_foreign_scope":
				_, err := f.db.Exec("UPDATE ai_messaging_operations SET organization_id=2 WHERE command_id=?", f.request.RequestID)
				mustMQ(t, err)
				query = "UPDATE ai_messaging_outbox SET organization_id=2 WHERE message_id=?"
			case "foreign_operation_orphan":
				_, err := f.db.Exec("DELETE FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID)
				mustMQ(t, err)
				query = "UPDATE ai_messaging_operations SET organization_id=2 WHERE command_id=?"
			case "foreign_outbox_orphan":
				_, err := f.db.Exec("DELETE FROM ai_messaging_operations WHERE command_id=?", f.request.RequestID)
				mustMQ(t, err)
				query = "UPDATE ai_messaging_outbox SET organization_id=2 WHERE message_id=?"
			case "body_hash":
				query = "UPDATE ai_messaging_outbox SET body_sha256=REPEAT('0',64) WHERE message_id=?"
			case "unknown_stage":
				query = "UPDATE ai_messaging_outbox SET stage='unrecognized' WHERE message_id=?"
			case "unknown_decision":
				query = "UPDATE ai_messaging_operations SET decision='unrecognized' WHERE command_id=?"
			case "broker_confirmed_without_receipt":
				query = "UPDATE ai_messaging_outbox SET stage='confirmed' WHERE message_id=?"
			}
			_, err := f.db.Exec(query, f.request.RequestID)
			mustMQ(t, err)
			if _, err = s.GetRuntime(t.Context(), 1, f.request.RequestID); !errors.Is(err, ErrMessagingLedgerIntegrity) {
				t.Fatal("invalid ledger returned statistics", err)
			}
			if _, err = s.RuntimeBacklog(t.Context(), 1); !errors.Is(err, ErrMessagingLedgerIntegrity) {
				t.Fatal("invalid ledger returned healthy backlog", err)
			}
			if _, err = ReadCurrentMessagingOrganization(t.Context(), f.db, 1); !errors.Is(err, ErrMessagingLedgerIntegrity) {
				t.Fatal("borrowed read hid an orphan/conflict/unknown ledger", err)
			}
		})
	}
}
