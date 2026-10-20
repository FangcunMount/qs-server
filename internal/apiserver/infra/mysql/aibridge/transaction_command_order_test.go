package aibridge

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

const orderAggregate = "b1941896-e7df-4b9d-9417-b80bd05276b9"
const orderSession = "c1941896-e7df-4b9d-9417-b80bd05276b9"
const orderCommand = "d1941896-e7df-4b9d-9417-b80bd05276b9"

func orderCommandFixture(t *testing.T) app.TransactionCommand {
	t.Helper()
	c, err := app.NewTransactionCommand(pb.MessagingKind_CHANGE, orderCommand, orderAggregate, app.OperationScope{OrganizationID: "7", SubjectID: "operator:9", ResourceID: orderSession}, &pb.MessagingBody{Value: &pb.MessagingBody_Change{Change: &pb.ChangeCommand{CommandId: orderCommand, SessionId: orderSession, Actor: &pb.Actor{OrgId: "7", SubjectId: "operator:9"}, ExpectedVersion: 1}}})
	require.NoError(t, err)
	return c
}
func orderDatabase(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	return db, mock
}

func TestCommandOrderPlanningDoesNotWriteOrAllocateBeforeHalves(t *testing.T) {
	db, mock := orderDatabase(t)
	c := orderCommandFixture(t)
	mock.ExpectQuery("SELECT next_sequence FROM ai_messaging_aggregates").WithArgs(orderAggregate).WillReturnError(sql.ErrNoRows)
	next, err := readCommandSequence(t.Context(), db, c, false)
	require.NoError(t, err)
	require.EqualValues(t, 1, next)
	// No Begin/Exec/Commit expectation: this is an optimistic snapshot, not a
	// successful submission, a lease, an Outbox or an authority mutation.
}

func TestCommandOrderWaitsForExactPredecessorAdmissionReceipt(t *testing.T) {
	for _, name := range []string{"pending", "held", "accepted", "rejected", "missing", "bad-kind", "bad-hash", "bad-command", "bad-decision", "missing-date", "wrong-session"} {
		t.Run(name, func(t *testing.T) {
			db, mock := orderDatabase(t)
			c := orderCommandFixture(t)
			mock.ExpectQuery("SELECT next_sequence FROM ai_messaging_aggregates").WithArgs(orderAggregate).WillReturnRows(sqlmock.NewRows([]string{"next_sequence"}).AddRow(uint64(2)))
			query := mock.ExpectQuery("SELECT command_id,kind,body_sha256,resource_id,decision,receipt_id,receipt,decided_at").WithArgs("7", orderAggregate, uint64(1))
			if name == "missing" {
				query.WillReturnError(sql.ErrNoRows)
				_, err := readCommandSequence(t.Context(), db, c, false)
				require.ErrorIs(t, err, app.ErrMessagingContract)
				return
			}
			hash := strings.Repeat("a", 64)
			kind := int32(pb.MessagingKind_START)
			decision := "accepted"
			var receiptID any = "e1941896-e7df-4b9d-9417-b80bd05276b9"
			var decided any = time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC)
			r := &pb.MessagingCommandReceipt{CommandId: orderAggregate, CommandBodySha256: hash, Decision: pb.MessagingDecision_ACCEPTED, OriginalReceipt: &pb.MessagingCommandReceipt_WorkflowReceipt{WorkflowReceipt: &pb.Receipt{SessionId: orderSession, Version: 1, Status: "running"}}}
			switch name {
			case "pending":
				decision = ""
				receiptID = nil
				decided = nil
			case "held":
				decision = "held"
				r.Decision = pb.MessagingDecision_HELD
			case "rejected":
				decision = "rejected"
				r.Decision = pb.MessagingDecision_REJECTED
				r.OriginalReceipt = nil
			case "bad-kind":
				kind = 99
			case "bad-hash":
				r.CommandBodySha256 = strings.Repeat("b", 64)
			case "bad-command":
				r.CommandId = orderCommand
			case "bad-decision":
				r.Decision = pb.MessagingDecision_REJECTED
			case "missing-date":
				decided = nil
			case "wrong-session":
				r.GetWorkflowReceipt().SessionId = "invalid"
			}
			var raw []byte
			if name != "pending" {
				var err error
				raw, err = proto.Marshal(&pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: r}})
				require.NoError(t, err)
			}
			query.WillReturnRows(sqlmock.NewRows([]string{"id", "kind", "hash", "resource", "decision", "receipt_id", "receipt", "decided"}).AddRow(orderAggregate, kind, hash, orderAggregate, decision, receiptID, raw, decided))
			next, err := readCommandSequence(t.Context(), db, c, false)
			switch name {
			case "accepted", "rejected":
				require.NoError(t, err)
				require.EqualValues(t, 2, next)
			case "pending", "held":
				require.ErrorIs(t, err, ErrCommandPredecessorPending)
			default:
				require.ErrorIs(t, err, app.ErrMessagingContract)
			}
		})
	}
}

func TestCommandOrderOriginalRootRejectsStaleFrozenSequence(t *testing.T) {
	db, mock := orderDatabase(t)
	c := orderCommandFixture(t)
	mock.ExpectBegin()
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	mock.ExpectExec("INSERT INTO ai_messaging_aggregates").WithArgs(orderAggregate).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT next_sequence FROM ai_messaging_aggregates.*FOR UPDATE").WithArgs(orderAggregate).WillReturnRows(sqlmock.NewRows([]string{"next_sequence"}).AddRow(uint64(1)))
	require.ErrorIs(t, guardCommandSequence(t.Context(), tx, c, 2), app.ErrConflict)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
}

func TestCommandOrderRequiresExactUniqueVisibleFullIndex(t *testing.T) {
	for _, name := range []string{"valid", "missing", "non-unique", "prefix", "wrong-column", "invisible"} {
		t.Run(name, func(t *testing.T) {
			db, mock := orderDatabase(t)
			rows := sqlmock.NewRows([]string{"ordinal", "column", "prefix", "non_unique", "type", "visible"})
			if name != "missing" {
				var prefix any
				nonUnique, visible, column := 0, "YES", "aggregate_sequence"
				if name == "prefix" {
					prefix = 10
				}
				if name == "non-unique" {
					nonUnique = 1
				}
				if name == "invisible" {
					visible = "NO"
				}
				if name == "wrong-column" {
					column = "command_id"
				}
				rows.AddRow(1, "aggregate_key", prefix, nonUnique, "BTREE", visible).AddRow(2, column, nil, 0, "BTREE", "YES")
			}
			mock.ExpectQuery("SELECT SEQ_IN_INDEX,COLUMN_NAME,SUB_PART,NON_UNIQUE,INDEX_TYPE,IS_VISIBLE").WillReturnRows(rows)
			err := requireCommandOrderIndex(context.Background(), db)
			if name == "valid" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, app.ErrMessagingContract)
			}
		})
	}
}
