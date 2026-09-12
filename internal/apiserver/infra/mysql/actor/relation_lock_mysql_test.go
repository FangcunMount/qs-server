package actor

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"os"
	"strings"
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	dbctx "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestRelationLockReadsCurrentStateAfterConcurrentUnbindMySQL(t *testing.T) {
	dsn := os.Getenv("QS_SCOPE_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_SCOPE_MYSQL_REQUIRED") == "1" {
			t.Fatal("QS_SCOPE_MYSQL_DSN required")
		}
		t.Skip("isolated MySQL not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
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
	require.NoError(t, db.Exec("CREATE TABLE clinician_relation(id BIGINT PRIMARY KEY,org_id BIGINT,testee_id BIGINT,clinician_id BIGINT,relation_type VARCHAR(50),source_type VARCHAR(50),is_active BOOLEAN,bound_at DATETIME,deleted_at DATETIME) ENGINE=InnoDB").Error)
	require.NoError(t, db.Exec("INSERT INTO clinician_relation VALUES(55,1,20,10,'primary','manual',1,'2026-01-01',NULL)").Error)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	transfer := db.WithContext(ctx).Begin()
	require.NoError(t, transfer.Error)
	defer transfer.Rollback()
	require.NoError(t, transfer.Exec("UPDATE clinician_relation SET is_active=0 WHERE id=55").Error)
	edit := db.WithContext(ctx).Begin()
	require.NoError(t, edit.Error)
	defer edit.Rollback()
	// Establish an old repeatable-read snapshot while the other update is uncommitted.
	stale, err := NewRelationRepository(db).FindByID(dbctx.WithTx(ctx, edit), 55)
	require.NoError(t, err)
	require.True(t, stale.IsActive())
	var connectionID int64
	require.NoError(t, edit.Raw("SELECT CONNECTION_ID()").Scan(&connectionID).Error)
	type result struct {
		item *domain.ClinicianTesteeRelation
		err  error
	}
	done := make(chan result, 1)
	go func() {
		item, err := NewRelationRepository(db).(domain.LockedRepository).FindByIDForUpdate(dbctx.WithTx(ctx, edit), 1, 20, 55)
		done <- result{item, err}
	}()
	// Observe an actual InnoDB wait, rather than treating scheduler delay as proof.
	require.Eventually(t, func() bool {
		var count int64
		err := db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.threads t ON t.THREAD_ID=w.REQUESTING_THREAD_ID WHERE t.PROCESSLIST_ID=?`, connectionID).Scan(&count).Error
		return err == nil && count > 0
	}, 4*time.Second, 20*time.Millisecond)
	require.NoError(t, transfer.Commit().Error)
	select {
	case result := <-done:
		require.NoError(t, result.err)
		require.False(t, result.item.IsActive(), "locking read must see committed unbind despite old snapshot")
	case <-ctx.Done():
		t.Fatal("edit did not resume after transfer commit")
	}
	// Both assignment queries must use current reads, even though this transaction
	// still has its earlier repeatable-read snapshot.
	assignmentReader := NewRelationRepository(db).(domain.AssignmentLockedRepository)
	_, err = assignmentReader.FindActivePrimaryByTesteeForUpdate(dbctx.WithTx(ctx, edit), 1, 20)
	require.Error(t, err)
	require.True(t, errors.IsCode(err, code.ErrUserNotFound))
	_, err = assignmentReader.FindActiveByTypesForUpdate(dbctx.WithTx(ctx, edit), 1, 10, 20, []domain.RelationType{domain.RelationTypePrimary})
	require.Error(t, err)
	require.True(t, errors.IsCode(err, code.ErrUserNotFound))
	require.NoError(t, edit.Commit().Error)
}
