package evaluation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestAssessmentRangeFollowsCurrentStoreBeforePagination(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE testee (id INTEGER PRIMARY KEY, org_id INTEGER, store_id INTEGER, deleted_at DATETIME)").Error)
	require.NoError(t, db.Exec("CREATE TABLE assessment (id INTEGER PRIMARY KEY, org_id INTEGER, testee_id INTEGER, deleted_at DATETIME)").Error)
	require.NoError(t, db.Exec("INSERT INTO testee VALUES (1,1,7,NULL),(2,1,8,NULL),(3,1,NULL,NULL),(4,2,7,NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO assessment VALUES (11,1,1,NULL),(12,1,1,NULL),(13,1,2,NULL),(14,1,3,NULL),(15,2,4,NULL)").Error)
	count := func(store uint64) int64 {
		q := applyAssessmentReadModelFilter(db.Table("assessment"), evaluationreadmodel.AssessmentFilter{OrgID: 1, RestrictToStoreScope: true, AllowedStoreIDs: []uint64{store}})
		var total int64
		require.NoError(t, q.Count(&total).Error)
		var ids []uint64
		require.NoError(t, q.Order("id").Limit(1).Pluck("id", &ids).Error)
		if total > 0 {
			require.Len(t, ids, 1)
		}
		return total
	}
	require.EqualValues(t, 2, count(7))
	require.EqualValues(t, 1, count(8))
	require.NoError(t, db.Exec("UPDATE testee SET store_id=8 WHERE id=1").Error)
	require.EqualValues(t, 0, count(7))
	require.EqualValues(t, 3, count(8))
	var total int64
	q := applyAssessmentReadModelFilter(db.Table("assessment"), evaluationreadmodel.AssessmentFilter{OrgID: 1, RestrictToStoreScope: true, AllAssignedStores: true})
	require.NoError(t, q.Count(&total).Error)
	require.EqualValues(t, 3, total, "unassigned and other company must remain excluded")
}
