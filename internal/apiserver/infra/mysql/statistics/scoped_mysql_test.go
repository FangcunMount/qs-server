package statistics

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// The runner supplies a disposable database, never a deployed database.
func TestScopedOverviewMySQL(t *testing.T) {
	dsn := os.Getenv("QS_SCOPE_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_SCOPE_MYSQL_REQUIRED") == "1" {
			t.Fatal("QS_SCOPE_MYSQL_DSN is required")
		}
		t.Skip("isolated MySQL DSN not configured")
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
	var name string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&name).Error)
	require.True(t, strings.HasPrefix(name, "qs_scope_test_"), "requires disposable qs_scope_test_ database")
	createScopedOverviewFixture(t, db)
	date := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec("INSERT INTO statistics_assessment_fact VALUES(1,1,1,NULL,'assessment_created',?,'Q','scale','S')", date).Error)
	require.NoError(t, db.Exec("INSERT INTO statistics_plan_fact(id,org_id,testee_id,fact_type,stat_date,plan_id,task_id,schedule_revision,schedule_planned_at,schedule_due_at) VALUES(1,1,1,'task_schedule_defined',?,1,1,1,?,?)", date, date, date).Error)
	store := NewReadStore(db, nil)
	read := func(id uint64) {
		t.Helper()
		result, err := store.ScopedOverview(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{id}}, date, date.AddDate(0, 0, 1), date.AddDate(0, 0, 1))
		require.NoError(t, err)
		require.EqualValues(t, 1, result.Metrics.TesteeCount)
		require.EqualValues(t, 1, result.Trends.Assessment.AssessmentCreated[0].Count)
		require.EqualValues(t, 1, result.Trends.PlanFulfillment.Due[0].Count)
	}
	read(7)
	require.NoError(t, db.Exec("UPDATE testee SET store_id=8 WHERE id=1").Error)
	result, err := store.ScopedOverview(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, date, date.AddDate(0, 0, 1), date.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Zero(t, result.Metrics.TesteeCount)
	require.Zero(t, result.Trends.PlanFulfillment.Due[0].Count)
	read(8)
}
