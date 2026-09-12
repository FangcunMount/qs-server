package statistics

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestScopedWindowCountsOnlyCurrentStoreAndRequestedDates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, sql := range []string{
		"CREATE TABLE testee(id INTEGER,org_id INTEGER,store_id INTEGER,deleted_at DATETIME)",
		"CREATE TABLE clinician(id INTEGER,org_id INTEGER,store_id INTEGER,deleted_at DATETIME)",
		"CREATE TABLE assessment_entry(id INTEGER,org_id INTEGER,clinician_id INTEGER,deleted_at DATETIME)",
		"INSERT INTO testee VALUES(1,1,7,NULL),(2,1,8,NULL),(3,1,NULL,NULL),(4,2,7,NULL)",
	} {
		require.NoError(t, db.Exec(sql).Error)
	}
	for _, table := range []string{"statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact"} {
		require.NoError(t, db.Exec("CREATE TABLE "+table+"(org_id INTEGER,testee_id INTEGER,entry_id INTEGER,fact_type TEXT,stat_date DATETIME)").Error)
	}
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 1)
	for _, id := range []int{1, 2, 3, 4} {
		org := 1
		if id == 4 {
			org = 2
		}
		for table, fact := range map[string]string{"statistics_access_fact": "intake_confirmed", "statistics_assessment_fact": "report_generated", "statistics_plan_fact": "task_completed"} {
			require.NoError(t, db.Exec("INSERT INTO "+table+" VALUES(?,?,NULL,?,?)", org, id, fact, from).Error)
		}
	}
	require.NoError(t, db.Exec("INSERT INTO statistics_assessment_fact VALUES(1,1,NULL,'report_generated',?)", to).Error)
	store := NewReadStore(db, nil)
	trends, err := store.scopedActivityTrends(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, from, from.AddDate(0, 0, 3))
	require.NoError(t, err)
	require.Len(t, trends.Assessment.ReportGenerated, 3)
	require.EqualValues(t, 1, trends.Assessment.ReportGenerated[0].Count)
	require.EqualValues(t, 1, trends.Assessment.ReportGenerated[1].Count)
	require.Zero(t, trends.Assessment.ReportGenerated[2].Count)
	require.EqualValues(t, 1, trends.PlanActivity.TaskCompleted[0].Count)
	require.Zero(t, trends.EnrolledTestees)

	result, err := store.scopedWindowMetrics(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, from, to)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.IntakeConfirmedCount)
	require.EqualValues(t, 1, result.WindowReportGeneratedCount)
	require.EqualValues(t, 1, result.TaskCompletedCount)
	require.Zero(t, result.WindowAssessmentCreatedCount)
	require.NoError(t, db.Exec("UPDATE testee SET store_id=8 WHERE id=1 AND org_id=1").Error)
	trends, err = store.scopedActivityTrends(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, from, to)
	require.NoError(t, err)
	require.Zero(t, trends.Assessment.ReportGenerated[0].Count)
	require.Zero(t, trends.PlanActivity.TaskCompleted[0].Count)

	result, err = store.scopedWindowMetrics(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, from, to)
	require.NoError(t, err)
	require.Zero(t, result.WindowReportGeneratedCount)
	require.Zero(t, result.TaskCompletedCount)
	result, err = store.scopedWindowMetrics(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{8}}, from, to)
	require.NoError(t, err)
	require.EqualValues(t, 2, result.WindowReportGeneratedCount)
	require.EqualValues(t, 2, result.TaskCompletedCount)
}
