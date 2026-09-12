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

func TestScopedOverviewComposesComponentsAndFailsWithoutPartialResult(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	createScopedOverviewFixture(t, db)
	date := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec("INSERT INTO statistics_assessment_fact VALUES(1,1,1,NULL,'assessment_created',?,'Q','scale','S')", date).Error)
	store := NewReadStore(db, nil)
	result, err := store.ScopedOverview(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, date, date.AddDate(0, 0, 1), date.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.EqualValues(t, 1, result.Metrics.TesteeCount)
	require.EqualValues(t, 1, result.Metrics.AssessmentCount)
	require.EqualValues(t, 1, result.Metrics.WindowAssessmentCreatedCount)
	require.Len(t, result.Trends.Assessment.AssessmentCreated, 1)
	require.EqualValues(t, 1, result.Trends.Assessment.AssessmentCreated[0].Count)
	require.Len(t, result.Trends.PlanFulfillment.Due, 1)
	require.Zero(t, result.Trends.PlanFulfillment.Due[0].Count)
	require.NoError(t, db.Exec("DROP TABLE statistics_plan_fact").Error)
	result, err = store.ScopedOverview(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, date, date.AddDate(0, 0, 1), date.AddDate(0, 0, 1))
	require.Error(t, err)
	require.Zero(t, result.Metrics.TesteeCount)
	require.Empty(t, result.Trends.Assessment.AssessmentCreated)
}

func createScopedOverviewFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, ddl := range []string{
		"CREATE TABLE testee(id INTEGER,org_id INTEGER,store_id INTEGER,deleted_at DATETIME)",
		"CREATE TABLE clinician(id INTEGER,org_id INTEGER,store_id INTEGER,is_active INTEGER,deleted_at DATETIME)",
		"CREATE TABLE assessment_entry(id INTEGER,org_id INTEGER,clinician_id INTEGER,is_active INTEGER,deleted_at DATETIME)",
		"CREATE TABLE plan_enrollment(org_id INTEGER,testee_id INTEGER,status TEXT,deleted_at DATETIME)",
		"CREATE TABLE statistics_access_fact(id INTEGER,org_id INTEGER,testee_id INTEGER,entry_id INTEGER,fact_type TEXT,stat_date DATETIME)",
		"CREATE TABLE statistics_assessment_fact(id INTEGER,org_id INTEGER,testee_id INTEGER,entry_id INTEGER,fact_type TEXT,stat_date DATETIME,questionnaire_code TEXT,model_kind TEXT,model_code TEXT)",
		"CREATE TABLE statistics_plan_fact(id INTEGER,org_id INTEGER,testee_id INTEGER,entry_id INTEGER,fact_type TEXT,stat_date DATETIME,plan_id INTEGER,task_id INTEGER,schedule_revision INTEGER,schedule_planned_at DATETIME,schedule_due_at DATETIME,task_status TEXT,completed_at DATETIME,planned_at DATETIME,due_at DATETIME)",
		"INSERT INTO testee VALUES(1,1,7,NULL)",
	} {
		require.NoError(t, db.Exec(ddl).Error)
	}
}
