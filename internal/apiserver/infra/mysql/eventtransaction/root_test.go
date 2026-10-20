package eventtransaction

import (
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestOriginalPlanRejectsWrongPrimaryFenceRootAndExpiredCallback(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		mock.ExpectClose()
		require.NoError(t, pool.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	}()
	db, err := gorm.Open(driver.New(driver.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	p, err := freeze(fixtureRequest("root-proof"), fixtureResolver{})
	require.NoError(t, err)
	e := transaction.Execution{Plan: p, Fence: 1, WriterEpoch: "m3-1"}
	proof, err := evidence.NewTransactional(standard.ReferenceFromMessage(p.Input().Branches[0].Message), e.References()[0], strings.Repeat("a", 64), time.Now().UTC())
	require.NoError(t, err)
	_, err = OriginalPlan(t.Context(), pool, proof)
	require.ErrorIs(t, err, transaction.ErrTransactionRequired)
	mock.ExpectBegin()
	tx := db.Begin()
	require.NoError(t, tx.Error)
	bound, closeRoot, err := bindOriginal(hostmysql.WithTx(t.Context(), tx), pool, e)
	require.NoError(t, err)
	actual, err := OriginalPlan(bound, pool, proof)
	require.NoError(t, err)
	require.Equal(t, p.Digest(), actual.Digest())
	_, err = OriginalPlan(bound, nil, proof)
	require.ErrorIs(t, err, transaction.ErrBinding)
	stale := *proof
	stale.TransactionReference, err = transaction.EncodeReference((transaction.Execution{Plan: p, Fence: 2, WriterEpoch: "m3-1"}).References()[0])
	require.NoError(t, err)
	_, err = OriginalPlan(bound, pool, &stale)
	require.ErrorIs(t, err, transaction.ErrBinding)
	_, err = OriginalPlan(hostmysql.WithTx(bound, db), pool, proof)
	require.ErrorIs(t, err, transaction.ErrBinding)
	closeRoot()
	_, err = OriginalPlan(bound, pool, proof)
	require.ErrorIs(t, err, transaction.ErrTransactionRequired)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback().Error)
}
