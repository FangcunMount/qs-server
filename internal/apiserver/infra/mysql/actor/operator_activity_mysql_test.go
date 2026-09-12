package actor

import (
	"context"
	"os"
	"strings"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestOperatorActivityRoundTripMySQL(t *testing.T) {
	dsn := os.Getenv("QS_SCOPE_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_SCOPE_MYSQL_REQUIRED") == "1" {
			t.Fatal("QS_SCOPE_MYSQL_DSN required")
		}
		t.Skip("isolated MySQL not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	var database string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&database).Error)
	require.True(t, strings.HasPrefix(database, "qs_scope_test_"), "disposable database required")
	require.NoError(t, db.AutoMigrate(&OperatorPO{}))
	// Existing installations retain the database default after this code change.
	require.NoError(t, db.Exec("ALTER TABLE operators ALTER COLUMN is_active SET DEFAULT 1").Error)
	repo := NewOperatorRepository(db)
	for i, active := range []bool{false, true} {
		op := domain.NewOperator(1, int64(800101+i), "activity round trip")
		if !active {
			require.NoError(t, domain.NewLifecycler().Deactivate(op))
		}
		require.NoError(t, repo.Save(context.Background(), op))
		var saved OperatorPO
		require.NoError(t, db.First(&saved, "user_id=?", op.UserID()).Error)
		require.Equal(t, active, saved.IsActive)
		loaded, err := repo.FindByUser(context.Background(), 1, op.UserID())
		require.NoError(t, err)
		require.Equal(t, active, loaded.IsActive())
	}
}
