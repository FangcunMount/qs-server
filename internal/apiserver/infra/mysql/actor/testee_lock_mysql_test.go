package actor

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	dbctx "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestTesteeEditWaitsForTransferMySQL(t *testing.T) {
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
	require.NoError(t, db.Exec("CREATE TABLE testee(id BIGINT PRIMARY KEY,org_id BIGINT,store_id BIGINT,store_version INT,name VARCHAR(80),deleted_at DATETIME) ENGINE=InnoDB").Error)
	require.NoError(t, db.Exec("INSERT INTO testee VALUES(1,1,7,1,'T',NULL)").Error)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	transfer := db.WithContext(ctx).Begin()
	require.NoError(t, transfer.Error)
	defer transfer.Rollback()
	require.NoError(t, transfer.Exec("UPDATE testee SET store_id=8,store_version=2 WHERE id=1").Error)
	edit := db.WithContext(ctx).Begin()
	require.NoError(t, edit.Error)
	defer edit.Rollback()
	var connectionID int64
	require.NoError(t, edit.Raw("SELECT CONNECTION_ID()").Scan(&connectionID).Error)
	type result struct {
		item *domain.Testee
		err  error
	}
	done := make(chan result, 1)
	go func() {
		item, err := NewTesteeRepository(db).(domain.LockedRepository).FindByIDForUpdate(dbctx.WithTx(ctx, edit), 1, 1)
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
		require.NotNil(t, result.item.StoreID())
		require.EqualValues(t, 8, *result.item.StoreID())
		require.EqualValues(t, 2, result.item.StoreVersion())
	case <-ctx.Done():
		t.Fatal("edit did not resume after transfer commit")
	}
	require.NoError(t, edit.Commit().Error)
}
