package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEventReceiverRefusesCallerRootBeforeAuthenticationOrFailureStorage(t *testing.T) {
	a, _, _ := commandArchiveFixture(t)
	r := &MessagingEventReceiver{DB: a.db, Transactions: &TransactionEvents{}}
	ctx := hostmysql.WithTx(t.Context(), &gorm.DB{})
	require.ErrorIs(t, r.Receive(ctx, []byte("invalid")), transaction.ErrTransactionRequired)
	require.ErrorIs(t, r.Failed(ctx, []byte("invalid")), transaction.ErrTransactionRequired)
	require.ErrorIs(t, r.Isolate(ctx, []byte("invalid"), "authentication_failed"), transaction.ErrTransactionRequired)
}

func eventExecutionFixture(t *testing.T, held bool) (app.FrozenIncomingEvent, transaction.Execution, app.FrozenMessagingArchive) {
	t.Helper()
	id, run := uuid.NewString(), uuid.NewString()
	b := &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationState{EvaluationState: &pb.EvaluationRuntimeState{RunId: run, OrganizationId: "7", EventSequence: 1, Version: 1, Status: "requested"}}}
	header, raw, err := app.PrepareMessaging(pb.MessagingKind_EVALUATION_STATE, id, run, "", b)
	require.NoError(t, err)
	header.OriginalOccurredAt = "2026-10-21T05:00:00+08:00"
	wrap := func(id string) []byte {
		wire, err := legacy.Encode(legacy.Envelope{UUID: id, Metadata: map[string]string{"secure_profile": protected.Profile}, Payload: []byte("opaque-unit-wire")}, legacy.Revision2)
		require.NoError(t, err)
		return wire
	}
	incoming, err := app.FreezeIncomingEvent(header, raw, wrap(id), "7")
	require.NoError(t, err)
	outcome := pb.MessagingEventAcknowledgement_STORED
	if held {
		outcome = pb.MessagingEventAcknowledgement_TECHNICALLY_HELD
	}
	ackID := uuid.NewString()
	ackHeader, ackBody, err := app.PrepareMessaging(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT, ackID, run, "", &pb.MessagingBody{Value: &pb.MessagingBody_EventAcknowledgement{EventAcknowledgement: &pb.MessagingEventAcknowledgement{EventId: id, EventBodySha256: header.BodySha256, EventKind: header.Kind, Outcome: outcome}}})
	require.NoError(t, err)
	ackHeader.OriginalOccurredAt = "2026-10-21T05:00:01+08:00"
	p, err := incoming.FreezeACK(&app.PreparedMessaging{Envelope: ackHeader, Body: ackBody, Wire: wrap(ackID), Topic: app.AcksTopic}, uuid.NewString())
	require.NoError(t, err)
	a, err := incoming.ReadACK(p)
	require.NoError(t, err)
	return incoming, transaction.Execution{Plan: p, Fence: 1, WriterEpoch: "test-epoch", Token: [32]byte{1}, LeaseUntil: time.Now().Add(time.Minute)}, a
}

func expectEventArchive(t *testing.T, mock sqlmock.Sqlmock, e transaction.Execution, write bool, state transaction.State) {
	t.Helper()
	b := e.Plan.Input().Branches[0]
	a, err := app.DecodeMessagingBranch(b)
	require.NoError(t, err)
	ek, bk := transaction.ExecutionKey("qs.mysql", e.Plan.Input().OperationID, e.WriterEpoch, e.Fence), transaction.BranchKey(b.ID)
	ref, err := transaction.EncodeReference(e.References()[0])
	require.NoError(t, err)
	args := []any{[]byte("qs-server"), []byte("qs-ai"), []byte(a.Envelope.MessageId), a.OrganizationID, b.Message.Input().Payload, b.Wire, ek[:], bk[:], ref}
	if write {
		mock.ExpectExec("INSERT INTO ai_messaging_archive").WithArgs(args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8]).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectQuery("SELECT organization_id,frozen_payload,first_wire,execution_key,branch_key,reference FROM ai_messaging_archive").WithArgs(args[0], args[1], args[2]).WillReturnRows(sqlmock.NewRows([]string{"org", "payload", "wire", "execution", "branch", "reference"}).AddRow(a.OrganizationID, b.Message.Input().Payload, b.Wire, ek[:], bk[:], ref))
	if !write {
		mock.ExpectQuery("SELECT w.reference,e.state,w.state FROM rm_tx_execution").WithArgs(ek[:], bk[:], []byte("test-primary"), 1).WillReturnRows(sqlmock.NewRows([]string{"ref", "exec", "branch"}).AddRow(ref, string(state), string(state)))
	}
}

func expectEventInbox(mock sqlmock.Sqlmock, incoming app.FrozenIncomingEvent, ack app.FrozenMessagingArchive, outcome, ackID string, insert bool) {
	h, raw, wire, _ := incoming.Input()
	if insert {
		mock.ExpectExec("INSERT INTO ai_messaging_inbox").WithArgs(h.Producer, h.MessageId, h.BodySha256, raw, messagingHash(wire), h.Kind, h.AggregateKey, ack.Envelope.MessageId, outcome).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectQuery("SELECT body_sha256,body,wire_sha256,kind,aggregate_key,ack_id,outcome FROM ai_messaging_inbox").WithArgs(h.Producer, h.MessageId).WillReturnRows(sqlmock.NewRows([]string{"hash", "body", "wire", "kind", "aggregate", "ack", "outcome"}).AddRow(h.BodySha256, raw, messagingHash(wire), int32(h.Kind), h.AggregateKey, ackID, outcome))
}

func TestEventOriginalRootCommitsInboxProjectionFirstACKAndWitness(t *testing.T) {
	for _, name := range []string{"stored", "held", "commit-reply-lost"} {
		t.Run(name, func(t *testing.T) {
			a, mock, _ := commandArchiveFixture(t)
			held := name == "held"
			incoming, e, ack := eventExecutionFixture(t, held)
			h, raw, _, _ := incoming.Input()
			s := &TransactionEvents{archive: a, store: NewMessagingStore()}
			mock.ExpectBegin()
			expectCommandGuard(t, mock, e) // must precede EVERY Inbox/business write
			attempts := uint64(0)
			outcome := "stored"
			if held {
				attempts, outcome = 8, "held"
			}
			mock.ExpectQuery("SELECT body_sha256,kind,aggregate_key,attempts.*FOR UPDATE").WithArgs(h.Producer, h.MessageId).WillReturnRows(sqlmock.NewRows([]string{"hash", "kind", "aggregate", "attempts"}).AddRow(h.BodySha256, int32(h.Kind), h.AggregateKey, attempts))
			expectEventInbox(mock, incoming, ack, outcome, ack.Envelope.MessageId, true)
			if !held {
				mock.ExpectExec("INSERT INTO ai_messaging_evaluation_states").WithArgs(h.AggregateKey, "7", []byte{}).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectQuery("SELECT CAST\\(organization_id AS CHAR\\),event_sequence,version,state.*FOR UPDATE").WithArgs(h.AggregateKey).WillReturnRows(sqlmock.NewRows([]string{"org", "sequence", "version", "state"}).AddRow("7", 0, 0, []byte{}))
				mock.ExpectExec("UPDATE ai_messaging_evaluation_states").WithArgs(uint64(1), int64(1), raw, h.AggregateKey).WillReturnResult(sqlmock.NewResult(0, 1))
			} // HELD has no projection query/Exec expectation.
			expectEventArchive(t, mock, e, true, transaction.Committed)
			ek := transaction.ExecutionKey("qs.mysql", e.Plan.Input().OperationID, e.WriterEpoch, e.Fence)
			mock.ExpectExec("UPDATE rm_tx_witness SET state='committed'").WithArgs(ek[:]).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("UPDATE rm_tx_execution SET state='committed'").WithArgs(ek[:], e.Token[:]).WillReturnResult(sqlmock.NewResult(0, 1))
			commit := mock.ExpectCommit()
			fault := errors.New("unit driver loses commit reply; not a network fault")
			if name == "commit-reply-lost" {
				commit.WillReturnError(fault)
			}
			state, err := s.eventRoot(t.Context(), incoming, e, ack)
			if name == "commit-reply-lost" {
				require.ErrorIs(t, err, fault)
				require.Equal(t, transaction.LocalUnknown, state)
			} else {
				require.NoError(t, err)
				require.Equal(t, transaction.LocalCommitted, state)
			}
		})
	}
}

func TestEventChangedDispositionRequiresActualRollbackAndExactAbort(t *testing.T) {
	a, mock, _ := commandArchiveFixture(t)
	incoming, e, ack := eventExecutionFixture(t, false)
	h, _, _, _ := incoming.Input()
	mock.ExpectBegin()
	expectCommandGuard(t, mock, e)
	mock.ExpectQuery("SELECT body_sha256,kind,aggregate_key,attempts.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"hash", "kind", "aggregate", "attempts"}).AddRow(h.BodySha256, int32(h.Kind), h.AggregateKey, 8))
	mock.ExpectRollback()
	mock.ExpectBegin()
	expectCommandBinding(mock, e)
	ek := transaction.ExecutionKey("qs.mysql", e.Plan.Input().OperationID, e.WriterEpoch, e.Fence)
	mock.ExpectExec("UPDATE rm_tx_execution SET state='aborted'").WithArgs(ek[:], e.Token[:]).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE rm_tx_witness SET state='aborted'").WithArgs(ek[:]).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	state, err := (&TransactionEvents{archive: a, store: NewMessagingStore()}).eventRoot(t.Context(), incoming, e, ack)
	require.ErrorIs(t, err, errEventOutcomeChanged)
	require.Equal(t, transaction.LocalAborted, state)
}

func TestEventHistoricalInboxNeverBecomesANewACKWitness(t *testing.T) {
	for _, name := range []string{"absent", "historical", "conflicting"} {
		t.Run(name, func(t *testing.T) {
			a, mock, _ := commandArchiveFixture(t)
			incoming, _, _ := eventExecutionFixture(t, false)
			h, raw, wire, _ := incoming.Input()
			query := mock.ExpectQuery("SELECT body_sha256,body,wire_sha256,kind,aggregate_key FROM ai_messaging_inbox").WithArgs(h.Producer, h.MessageId)
			if name == "absent" {
				query.WillReturnError(sql.ErrNoRows)
			} else {
				hash := h.BodySha256
				if name == "conflicting" {
					hash = "different"
				}
				query.WillReturnRows(sqlmock.NewRows([]string{"hash", "body", "wire", "kind", "aggregate"}).AddRow(hash, raw, messagingHash(wire), int32(h.Kind), h.AggregateKey))
			}
			err := (&TransactionEvents{archive: a}).refuseHistoricalInbox(t.Context(), incoming)
			switch name {
			case "absent":
				require.NoError(t, err)
			case "historical":
				require.ErrorIs(t, err, ErrLegacyEventResponsibility)
			default:
				require.ErrorIs(t, err, app.ErrConflict)
			}
		})
	}
}

func TestEventCommittedRedeliveryUsesFirstACKWithoutProjectionOrHalf(t *testing.T) {
	for _, name := range []string{"duplicate", "lost-reply-recovery", "prepared", "wrong-inbox-ack"} {
		t.Run(name, func(t *testing.T) {
			a, mock, _ := commandArchiveFixture(t)
			incoming, e, ack := eventExecutionFixture(t, false)
			state := transaction.Committed
			if name == "prepared" {
				state = transaction.Prepared
			}
			expectEventArchive(t, mock, e, false, state)
			if name != "prepared" {
				id := ack.Envelope.MessageId
				if name == "wrong-inbox-ack" {
					id = uuid.NewString()
				}
				expectEventInbox(mock, incoming, ack, "stored", id, false)
				if name == "duplicate" {
					mock.ExpectBegin()
					mock.ExpectExec("INSERT INTO ai_messaging_observations").WithArgs("duplicate_event").WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectCommit()
				}
			}
			err := (&TransactionEvents{archive: a}).verifyCommittedEvent(context.Background(), incoming, e, name == "duplicate")
			if name == "prepared" || name == "wrong-inbox-ack" {
				require.ErrorIs(t, err, app.ErrMessagingContract)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
