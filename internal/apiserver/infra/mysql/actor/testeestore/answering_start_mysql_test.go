package testeestore_test

import (
	"context"
	"errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testeestore"
	startapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answeringstart"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	start "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answeringstart"
	sheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	repo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor/testeestore"
	startrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/survey/answeringstart"
	dbctx "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"os"
	"testing"
	"time"
)

type startAdmission struct{}

func (startAdmission) ValidateStart(context.Context, *start.Intent) error { return nil }
func TestMySQLAnsweringStartTransferAndRollback(t *testing.T) {
	db, transfer, ctx := fixture(t)
	raw, e := os.ReadFile("../../../../../pkg/migration/migrations/mysql/000080_answering_start.up.sql")
	require.NoError(t, e)
	require.NoError(t, db.Exec(string(raw)).Error)
	_, e = transfer.AssignInitial(ctx, app.Actor{OrgID: 7, UserID: 9}, 10, change(1, 1, "first"))
	require.NoError(t, e)
	runner := transaction.RunnerFunc(func(c context.Context, fn func(context.Context) error) error {
		return db.WithContext(c).Transaction(func(tx *gorm.DB) error { return fn(dbctx.WithTx(c, tx)) })
	})
	starts := startrepo.NewRepository(db)
	owners := repo.NewRepository(db)
	service := startapp.NewService(starts, runner, owners, startAdmission{})
	intent := start.Intent{OrgID: 7, UserID: 9, TesteeID: 10, RequestKey: "start-before-transfer", QuestionnaireCode: "Q", QuestionnaireVersion: "1", Origin: sheet.OriginRef{Type: sheet.OriginTypeSelfService}}
	// Hold the same ownership lock after start insertion: transfer must wait for commit.
	locked, release := make(chan struct{}), make(chan struct{})
	guarded := transaction.RunnerFunc(func(c context.Context, fn func(context.Context) error) error {
		return runner.WithinTransaction(c, func(txctx context.Context) error {
			if err := fn(txctx); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	})
	firstService := startapp.NewService(starts, guarded, owners, startAdmission{})
	firstDone := make(chan error, 1)
	go func() { _, err := firstService.Start(ctx, intent); firstDone <- err }()
	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		t.Fatal("start did not lock")
	}
	transferDone := make(chan error, 1)
	go func() {
		_, err := transfer.Transfer(ctx, app.Actor{OrgID: 7, UserID: 9}, 10, change(2, 2, "move"))
		transferDone <- err
	}()
	select {
	case err := <-transferDone:
		t.Fatalf("transfer bypassed start lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-transferDone)
	replay, e := service.Start(ctx, intent)
	require.NoError(t, e)
	require.False(t, replay.Created)
	require.EqualValues(t, 1, *replay.Record.Context().StoreID())
	intent.RequestKey = "after-transfer"
	second, e := service.Start(ctx, intent)
	require.NoError(t, e)
	require.EqualValues(t, 2, *second.Record.Context().StoreID())
	failed := transaction.RunnerFunc(func(c context.Context, fn func(context.Context) error) error {
		return runner.WithinTransaction(c, func(txctx context.Context) error {
			if err := fn(txctx); err != nil {
				return err
			}
			return errors.New("rollback injected")
		})
	})
	intent.RequestKey = "failed-transaction"
	_, e = startapp.NewService(starts, failed, owners, startAdmission{}).Start(ctx, intent)
	require.Error(t, e)
	missing, e := starts.FindRequest(ctx, 7, 9, intent.RequestKey)
	require.NoError(t, e)
	require.Nil(t, missing)
	down, e := os.ReadFile("../../../../../pkg/migration/migrations/mysql/000080_answering_start.down.sql")
	require.NoError(t, e)
	require.Error(t, db.Exec(string(down)).Error, "accepted starts must block destructive rollback")
	kept, e := starts.FindRequest(ctx, 7, 9, "start-before-transfer")
	require.NoError(t, e)
	require.NotNil(t, kept)

}
