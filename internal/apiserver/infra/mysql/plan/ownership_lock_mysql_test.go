package plan

import (
	"context"
	"errors"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	dbctx "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/stretchr/testify/require"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
	"strings"
	"testing"
	"time"
)

type ownershipScope struct{ store uint64 }

func (s ownershipScope) ResolveStoreRange(context.Context, int64, int64, string, string) (authz.StoreRange, error) {
	return authz.StoreRange{StoreIDs: []uint64{s.store}}, nil
}
func (ownershipScope) ValidateTesteeStoreAccess(context.Context, int64, int64, uint64, string, string) error {
	return errors.New("must use locked ownership")
}

type ownershipWriteCommand struct {
	planapp.PlanCommandService
	fail bool
}

func (s ownershipWriteCommand) EnrollTestee(ctx context.Context, _ planapp.EnrollTesteeDTO) (*planapp.EnrollmentResult, error) {
	tx, err := dbctx.RequireTx(ctx)
	if err != nil {
		return nil, err
	}
	if err := tx.Exec("INSERT INTO scope_write_probe VALUES(1)").Error; err != nil {
		return nil, err
	}
	if s.fail {
		return nil, errors.New("injected command failure")
	}
	return &planapp.EnrollmentResult{}, nil
}

func TestPlanEnrollmentOwnershipConcurrencyMySQL(t *testing.T) {
	dsn := os.Getenv("QS_SCOPE_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_SCOPE_MYSQL_REQUIRED") == "1" {
			t.Fatal("QS_SCOPE_MYSQL_DSN required")
		}
		t.Skip("isolated MySQL not configured")
	}
	db, err := gorm.Open(mysqlDriver.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	var database string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&database).Error)
	require.True(t, strings.HasPrefix(database, "qs_scope_test_"), "disposable database required")
	require.NoError(t, db.Exec("CREATE TABLE testee(id BIGINT PRIMARY KEY,org_id BIGINT,store_id BIGINT,deleted_at DATETIME) ENGINE=InnoDB").Error)
	require.NoError(t, db.Exec("CREATE TABLE scope_write_probe(id BIGINT PRIMARY KEY) ENGINE=InnoDB").Error)
	require.NoError(t, db.Exec("INSERT INTO testee VALUES(1,1,7,NULL)").Error)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ctx = actorctx.WithOperatorOrgID(actorctx.WithGrantingUserID(ctx, 2), 1)
	ctx = authz.WithSnapshot(ctx, &authz.Snapshot{Permissions: []authz.Permission{{Resource: authz.EvaluationPlanResource, Action: "enroll", Mode: authz.AuthorizationModeUnconditional}}})
	transfer := db.WithContext(ctx).Begin()
	require.NoError(t, transfer.Error)
	defer transfer.Rollback()
	require.NoError(t, transfer.Exec("UPDATE testee SET store_id=8 WHERE id=1").Error)
	connection := make(chan int64, 1)
	uow := dbctx.NewUnitOfWork(db)
	runner := transaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		return uow.WithinTransaction(ctx, func(txctx context.Context) error {
			tx, err := dbctx.RequireTx(txctx)
			if err != nil {
				return err
			}
			var id int64
			if err := tx.Raw("SELECT CONNECTION_ID()").Scan(&id).Error; err != nil {
				return err
			}
			select {
			case connection <- id:
			default:
			}
			return fn(txctx)
		})
	})
	build := func(store uint64, fail bool) planapp.PlanCommandService {
		return planapp.NewOperatorCommandService(ownershipWriteCommand{fail: fail}, ownershipScope{store: store}, nil, planapp.OperatorMutationDependencies{Transaction: runner, Ownership: OwnershipLocker{}})
	}
	done := make(chan error, 1)
	go func() {
		_, err := build(7, false).EnrollTestee(ctx, planapp.EnrollTesteeDTO{OrgID: 1, TesteeID: "1"})
		done <- err
	}()
	var connectionID int64
	select {
	case connectionID = <-connection:
	case <-ctx.Done():
		t.Fatal("enrollment did not enter transaction")
	}
	require.Eventually(t, func() bool {
		var count int64
		err := db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.threads t ON t.THREAD_ID=w.REQUESTING_THREAD_ID WHERE t.PROCESSLIST_ID=?`, connectionID).Scan(&count).Error
		return err == nil && count > 0
	}, 4*time.Second, 20*time.Millisecond)
	require.NoError(t, transfer.Commit().Error)
	select {
	case err := <-done:
		require.True(t, cberrors.IsCode(err, code.ErrPermissionDenied), "old store must receive permission denial")
	case <-ctx.Done():
		t.Fatal("enrollment did not resume")
	}
	countWrites := func() int64 {
		var count int64
		require.NoError(t, db.Table("scope_write_probe").Count(&count).Error)
		return count
	}
	require.Zero(t, countWrites(), "old store must not write after transfer")
	_, err = build(8, true).EnrollTestee(ctx, planapp.EnrollTesteeDTO{OrgID: 1, TesteeID: "1"})
	require.ErrorContains(t, err, "injected command failure")
	require.Zero(t, countWrites(), "delegate write must roll back with ownership transaction")
	_, err = build(8, false).EnrollTestee(ctx, planapp.EnrollTesteeDTO{OrgID: 1, TesteeID: "1"})
	require.NoError(t, err)
	require.EqualValues(t, 1, countWrites(), "new store can commit")
}
