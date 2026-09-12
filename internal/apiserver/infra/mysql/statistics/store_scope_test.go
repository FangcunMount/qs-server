package statistics

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestScopedStatisticsFactsFollowTesteeInsteadOfHistoricalClinician(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, sql := range []string{
		"CREATE TABLE testee(id INTEGER,org_id INTEGER,store_id INTEGER,deleted_at DATETIME)",
		"CREATE TABLE clinician(id INTEGER,org_id INTEGER,store_id INTEGER,deleted_at DATETIME)",
		"CREATE TABLE assessment_entry(id INTEGER,org_id INTEGER,clinician_id INTEGER,deleted_at DATETIME)",
		"CREATE TABLE statistics_access_fact(id INTEGER,org_id INTEGER,testee_id INTEGER,entry_id INTEGER,fact_type TEXT)",
		"CREATE TABLE statistics_assessment_fact(id INTEGER,org_id INTEGER,testee_id INTEGER,entry_id INTEGER,fact_type TEXT)",
		"INSERT INTO testee VALUES(1,1,7,NULL),(2,1,NULL,NULL),(3,2,7,NULL)",
		"INSERT INTO clinician VALUES(10,1,7,NULL)",
		"INSERT INTO assessment_entry VALUES(20,1,10,NULL)",
		"INSERT INTO statistics_access_fact VALUES(1,1,1,20,'intake_confirmed'),(2,1,NULL,20,'entry_opened'),(3,1,NULL,20,'intake_confirmed'),(4,2,NULL,20,'entry_opened')",
		"INSERT INTO statistics_assessment_fact VALUES(1,1,1,20,'assessment_created'),(2,1,2,20,'assessment_created'),(3,2,3,20,'assessment_created')",
	} {
		require.NoError(t, db.Exec(sql).Error)
	}
	store := NewReadStore(db, nil)
	count := func(table string, id uint64) int64 {
		var n int64
		require.NoError(t, store.scopedFacts(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{id}}, table).Count(&n).Error)
		return n
	}
	require.EqualValues(t, 1, count("statistics_assessment_fact", 7))
	require.EqualValues(t, 2, count("statistics_access_fact", 7))
	require.NoError(t, db.Exec("UPDATE testee SET store_id=8 WHERE id=1 AND org_id=1").Error)
	require.EqualValues(t, 0, count("statistics_assessment_fact", 7))
	require.EqualValues(t, 1, count("statistics_assessment_fact", 8))
	require.EqualValues(t, 1, count("statistics_access_fact", 7), "only anonymous entry opening stays with entry")
	require.EqualValues(t, 1, count("statistics_access_fact", 8), "intake follows current Testee")
}
