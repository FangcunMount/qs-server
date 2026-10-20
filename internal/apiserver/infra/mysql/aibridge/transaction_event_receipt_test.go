package aibridge

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

func TestTransactionReceiptBorrowsOriginalRootWithOnePoolSlot(t *testing.T) {
	for _, name := range []string{"committed", "prepared", "new-plan-without-archive", "historical-absence"} {
		t.Run(name, func(t *testing.T) {
			a, mock, e := commandArchiveFixture(t)
			a.db.SetMaxOpenConns(1)
			c, archive, err := app.ReadTransactionCommand(e.Plan)
			require.NoError(t, err)
			mock.ExpectBegin()
			tx, err := a.db.BeginTx(t.Context(), nil)
			require.NoError(t, err)
			if name == "committed" || name == "prepared" {
				state := transaction.Committed
				if name == "prepared" {
					state = transaction.Prepared
				}
				expectReadCommandArchive(t, mock, e, state)
			} else {
				mock.ExpectQuery("SELECT organization_id,frozen_payload,first_wire,execution_key,branch_key,reference FROM ai_messaging_archive").WillReturnError(sql.ErrNoRows)
				request := mock.ExpectQuery("SELECT owner,scope,idempotency_key,input_hash FROM rm_tx_request")
				if name == "historical-absence" {
					request.WillReturnError(sql.ErrNoRows)
				} else {
					q := transaction.RequestFor(e.Plan)
					request.WillReturnRows(sqlmock.NewRows([]string{"owner", "scope", "key", "hash"}).AddRow(q.Owner, q.Scope, q.Key, q.InputHash[:]))
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
			defer cancel()
			store := NewMessagingStore()
			store.Archive = a
			modern, err := store.transactionCommandReceipt(ctx, tx, c.ID(), "7", orderSession, orderAggregate, c.Kind(), archive.Envelope.BodySha256)
			switch name {
			case "committed":
				require.NoError(t, err)
				require.True(t, modern)
			case "prepared", "new-plan-without-archive":
				require.ErrorIs(t, err, app.ErrMessagingContract)
				require.True(t, modern)
			case "historical-absence":
				require.NoError(t, err)
				require.False(t, modern)
			}
			mock.ExpectRollback()
			require.NoError(t, tx.Rollback())
		})
	}
}
