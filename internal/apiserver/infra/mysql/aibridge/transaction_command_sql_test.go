package aibridge

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

func commandArchiveFixture(t *testing.T) (*TransactionArchive, sqlmock.Sqlmock, transaction.Execution) {
	t.Helper()
	db, mock := orderDatabase(t)
	e := commandExecutionFixture(t)
	a, err := NewTransactionArchive(db, transaction.Binding{Owner: "qs.mysql", StoreID: "test-primary", WriterEpoch: e.WriterEpoch})
	require.NoError(t, err)
	return a, mock, e
}

func expectCommandBinding(mock sqlmock.Sqlmock, e transaction.Execution) {
	mock.ExpectQuery("SELECT store_id,writer_epoch,schema_version FROM rm_tx_binding").WithArgs([]byte("qs.mysql")).WillReturnRows(sqlmock.NewRows([]string{"store", "epoch", "schema"}).AddRow("test-primary", e.WriterEpoch, 1))
}

func expectCommandGuard(t *testing.T, mock sqlmock.Sqlmock, e transaction.Execution) {
	t.Helper()
	ek := transaction.ExecutionKey("qs.mysql", e.Plan.Input().OperationID, e.WriterEpoch, e.Fence)
	bk := transaction.BranchKey(e.Plan.Input().Branches[0].ID)
	plan, err := transaction.EncodePlan(e.Plan)
	require.NoError(t, err)
	ref, err := transaction.EncodeReference(e.References()[0])
	require.NoError(t, err)
	expectCommandBinding(mock, e)
	mock.ExpectExec("UPDATE rm_tx_execution SET guard_version=guard_version\\+1").WithArgs(ek[:], e.Token[:]).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT plan FROM rm_tx_execution").WithArgs(ek[:]).WillReturnRows(sqlmock.NewRows([]string{"plan"}).AddRow(plan))
	mock.ExpectQuery("SELECT reference,state FROM rm_tx_witness.*FOR UPDATE").WithArgs(ek[:], bk[:]).WillReturnRows(sqlmock.NewRows([]string{"reference", "state"}).AddRow(ref, "prepared"))
}

func TestCommandOriginalSQLRootAtomicallyWritesFactsArchiveOperationAndWitness(t *testing.T) {
	for _, name := range []string{"committed", "commit-reply-lost"} {
		t.Run(name, func(t *testing.T) {
			a, mock, e := commandArchiveFixture(t)
			c, archive, err := app.ReadTransactionCommand(e.Plan)
			require.NoError(t, err)
			ek := transaction.ExecutionKey("qs.mysql", c.ID(), e.WriterEpoch, e.Fence)
			bk := transaction.BranchKey(c.ID())
			ref, err := transaction.EncodeReference(e.References()[0])
			require.NoError(t, err)
			mock.ExpectBegin()
			expectCommandGuard(t, mock, e)
			mock.ExpectExec("INSERT INTO ai_messaging_aggregates").WithArgs(orderAggregate).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery("SELECT next_sequence FROM ai_messaging_aggregates.*FOR UPDATE").WithArgs(orderAggregate).WillReturnRows(sqlmock.NewRows([]string{"next"}).AddRow(1))
			mock.ExpectQuery("SELECT closed,revision,updated_at FROM ai_messaging_admission.*LOCK IN SHARE MODE").WillReturnRows(sqlmock.NewRows([]string{"closed", "revision", "updated"}).AddRow(false, 1, time.Now()))
			original, _, err := encode(app.Start{RequestID: orderAggregate, Actor: app.Actor{OrgID: "7", SubjectID: "operator:9"}})
			require.NoError(t, err)
			mock.ExpectQuery("SELECT CAST\\(payload AS BINARY\\),session_id FROM ai_bridge_requests.*FOR UPDATE").WithArgs(orderAggregate).WillReturnRows(sqlmock.NewRows([]string{"payload", "session"}).AddRow(original, orderSession))
			mock.ExpectExec("INSERT INTO ai_messaging_operations").WithArgs(c.ID(), archive.Envelope.Kind, archive.Envelope.BodySha256, "7", "operator:9", orderSession, orderAggregate, uint64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("UPDATE ai_messaging_aggregates SET next_sequence=next_sequence\\+1.*AND next_sequence=\\?").WithArgs(orderAggregate, uint64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
			branch := e.Plan.Input().Branches[0]
			payload := branch.Message.Input().Payload
			mock.ExpectExec("INSERT INTO ai_messaging_archive").WithArgs([]byte("qs-server"), []byte("qs-ai"), []byte(c.ID()), uint64(7), payload, branch.Wire, ek[:], bk[:], ref).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT organization_id,frozen_payload,first_wire,execution_key,branch_key,reference FROM ai_messaging_archive").WithArgs([]byte("qs-server"), []byte("qs-ai"), []byte(c.ID())).WillReturnRows(sqlmock.NewRows([]string{"org", "payload", "wire", "execution", "branch", "reference"}).AddRow(7, payload, branch.Wire, ek[:], bk[:], ref))
			mock.ExpectExec("UPDATE rm_tx_witness SET state='committed'").WithArgs(ek[:]).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("UPDATE rm_tx_execution SET state='committed'").WithArgs(ek[:], e.Token[:]).WillReturnResult(sqlmock.NewResult(0, 1))
			fault := errors.New("database commit response lost")
			commit := mock.ExpectCommit()
			if name == "commit-reply-lost" {
				commit.WillReturnError(fault)
			}
			state, err := a.commandRoot(t.Context(), e)
			if name == "committed" {
				require.NoError(t, err)
				require.Equal(t, transaction.LocalCommitted, state)
			} else {
				require.ErrorIs(t, err, fault)
				require.Equal(t, transaction.LocalUnknown, state)
			}
		})
	}
}

func TestCommandOriginalSQLRootNeedsRollbackAndPersistedAbortForAborted(t *testing.T) {
	for _, name := range []string{"aborted", "rollback-failed", "abort-failed", "abort-not-owned"} {
		t.Run(name, func(t *testing.T) {
			a, mock, e := commandArchiveFixture(t)
			ek := transaction.ExecutionKey("qs.mysql", e.Plan.Input().OperationID, e.WriterEpoch, e.Fence)
			mock.ExpectBegin()
			expectCommandGuard(t, mock, e)
			mock.ExpectExec("INSERT INTO ai_messaging_aggregates").WithArgs(orderAggregate).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery("SELECT next_sequence FROM ai_messaging_aggregates.*FOR UPDATE").WithArgs(orderAggregate).WillReturnRows(sqlmock.NewRows([]string{"next"}).AddRow(0))
			rollback := mock.ExpectRollback()
			if name == "rollback-failed" {
				rollback.WillReturnError(errors.New("rollback response lost"))
			} else {
				mock.ExpectBegin()
				expectCommandBinding(mock, e)
				abort := mock.ExpectExec("UPDATE rm_tx_execution SET state='aborted'").WithArgs(ek[:], e.Token[:])
				switch name {
				case "abort-failed":
					abort.WillReturnError(sql.ErrConnDone)
					mock.ExpectRollback()
				case "abort-not-owned":
					abort.WillReturnResult(sqlmock.NewResult(0, 0))
					mock.ExpectRollback()
				default:
					abort.WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectExec("UPDATE rm_tx_witness SET state='aborted'").WithArgs(ek[:]).WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectCommit()
				}
			}
			state, err := a.commandRoot(t.Context(), e)
			require.ErrorIs(t, err, app.ErrMessagingContract)
			if name == "aborted" {
				require.Equal(t, transaction.LocalAborted, state)
			} else {
				require.Equal(t, transaction.LocalUnknown, state)
			}
		})
	}
}
