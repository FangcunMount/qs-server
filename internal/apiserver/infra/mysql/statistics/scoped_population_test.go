package statistics

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestScopedPopulationExcludesForeignAndUnassignedSubjects(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, sql := range []string{
		"CREATE TABLE testee(id INTEGER,org_id INTEGER,store_id INTEGER,deleted_at DATETIME)",
		"CREATE TABLE clinician(id INTEGER,org_id INTEGER,store_id INTEGER,is_active INTEGER,deleted_at DATETIME)",
		"CREATE TABLE assessment_entry(id INTEGER,org_id INTEGER,clinician_id INTEGER,is_active INTEGER,deleted_at DATETIME)",
		"CREATE TABLE plan_enrollment(org_id INTEGER,testee_id INTEGER,status TEXT,deleted_at DATETIME)",
		"CREATE TABLE statistics_assessment_fact(org_id INTEGER,testee_id INTEGER,fact_type TEXT,questionnaire_code TEXT,model_kind TEXT,model_code TEXT)",
		"INSERT INTO testee VALUES(1,1,7,NULL),(2,1,8,NULL),(3,1,NULL,NULL),(4,2,7,NULL)",
		"INSERT INTO clinician VALUES(10,1,7,1,NULL),(11,1,8,1,NULL),(12,2,7,1,NULL)",
		"INSERT INTO assessment_entry VALUES(20,1,10,1,NULL),(21,1,11,1,NULL),(22,2,12,1,NULL)",
		"INSERT INTO plan_enrollment VALUES(1,1,'active',NULL),(1,2,'active',NULL),(1,3,'active',NULL)",
		"INSERT INTO statistics_assessment_fact VALUES(1,1,'assessment_created','Q1','scale','S1'),(1,1,'report_generated','Q1','scale','S1'),(1,2,'assessment_created','Q2','scale','S2'),(1,3,'assessment_created','Q3','scale','S3'),(2,4,'assessment_created','Q4','scale','S4')",
	} {
		require.NoError(t, db.Exec(sql).Error)
	}
	store := NewReadStore(db, nil)
	asOf := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec("ALTER TABLE statistics_assessment_fact ADD COLUMN stat_date DATETIME").Error)
	require.NoError(t, db.Exec("INSERT INTO statistics_assessment_fact(org_id,testee_id,fact_type,questionnaire_code) VALUES(1,1,'answersheet_submitted','Q1')").Error)
	require.NoError(t, db.Exec("UPDATE statistics_assessment_fact SET stat_date=?", asOf).Error)
	refs := []app.ScopedContentRef{{ContentRef: app.ContentRef{Kind: "questionnaire", Code: "Q1"}, Stores: authz.StoreRange{StoreIDs: []uint64{7}}}, {ContentRef: app.ContentRef{Kind: "scale", Code: "S2"}, Stores: authz.StoreRange{StoreIDs: []uint64{8}}}}
	content, err := store.ScopedContentBatch(context.Background(), 1, asOf, refs)
	require.NoError(t, err)
	require.Len(t, content, 2)
	require.EqualValues(t, 1, content[0].TotalSubmissions)
	require.EqualValues(t, 1, content[1].TotalSubmissions)

	result, err := store.scopedPopulationMetrics(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}})
	require.NoError(t, err)
	require.EqualValues(t, 1, result.TesteeCount)
	require.EqualValues(t, 1, result.ClinicianCount)
	require.EqualValues(t, 1, result.ActiveEntryCount)
	require.EqualValues(t, 1, result.AssessmentCount)
	require.EqualValues(t, 1, result.ReportCount)
	require.EqualValues(t, 2, result.ContentCount)
	require.EqualValues(t, 1, result.ActiveEnrollmentCount)
	require.NoError(t, db.Exec("UPDATE testee SET store_id=8 WHERE id=1").Error)
	content, err = store.ScopedContentBatch(context.Background(), 1, asOf, refs)
	require.NoError(t, err)
	require.Zero(t, content[0].TotalSubmissions)

	result, err = store.scopedPopulationMetrics(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}})
	require.NoError(t, err)
	require.Zero(t, result.TesteeCount)
	require.Zero(t, result.AssessmentCount)
	require.Zero(t, result.ContentCount)
	require.EqualValues(t, 1, result.ClinicianCount, "doctor remains in original store")
}
