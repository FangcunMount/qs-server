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

func TestScopedClinicianAnalysisMySQL(t *testing.T) {
	dsn := os.Getenv("QS_SCOPE_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_SCOPE_MYSQL_REQUIRED") == "1" {
			t.Fatal("QS_SCOPE_MYSQL_DSN is required")
		}
		t.Skip("isolated MySQL DSN not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var name string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&name).Error)
	require.True(t, strings.HasPrefix(name, "qs_scope_test_"), "requires isolated database")
	for _, table := range []string{"clinician", "testee", "assessment_entry", "clinician_relation", "statistics_access_fact", "statistics_assessment_fact"} {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS "+table).Error)
	}
	createScopedClinicianFixture(t, db)
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec("INSERT INTO statistics_assessment_fact VALUES(1,10,1,20,'report_generated',?),(1,10,2,20,'report_generated',?)", from, from).Error)
	reader := NewReadStore(db, nil)
	items, total, summary, err := reader.ScopedClinicianAnalysis(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, from, from.AddDate(0, 0, 1), 1, 1)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.EqualValues(t, 2, total)
	require.EqualValues(t, 2, summary.ClinicianCount)
	require.EqualValues(t, 1, summary.ReportGeneratedCount)
	visible, err := reader.ScopedAnalysisClinicianVisible(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, 10)
	require.NoError(t, err)
	require.True(t, visible)
	require.NoError(t, db.Exec("UPDATE clinician SET store_id=8 WHERE id=10").Error)
	visible, err = reader.ScopedAnalysisClinicianVisible(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, 10)
	require.NoError(t, err)
	require.False(t, visible)
	items, total, summary, err = reader.ScopedClinicianAnalysis(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, from, from.AddDate(0, 0, 1), 1, 1)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.EqualValues(t, 11, items[0].ID)
	require.EqualValues(t, 1, total)
	require.Zero(t, summary.ReportGeneratedCount)
	population, err := reader.scopedFacts(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, "statistics_assessment_fact").Rows()
	require.NoError(t, err)
	defer func() { require.NoError(t, population.Close()) }()
	require.True(t, population.Next(), "subject history stays in original store after doctor transfer")
}
