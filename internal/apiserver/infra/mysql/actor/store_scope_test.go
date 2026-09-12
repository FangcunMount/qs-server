package actor

import (
	"context"
	cbErrors "github.com/FangcunMount/component-base/pkg/errors"
	rm "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestCurrentStoreSelectorRetainsCompanyAndUnassignedBoundary(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE testee (id INTEGER PRIMARY KEY, org_id INTEGER, store_id INTEGER, deleted_at DATETIME)").Error)
	require.NoError(t, db.Exec("INSERT INTO testee VALUES (1,1,7,NULL),(2,1,8,NULL),(3,1,NULL,NULL),(4,2,7,NULL),(5,1,7,'2026-01-01')").Error)
	reader := NewReadModel(db).(rm.TesteeStoreSelector)
	ctx := context.Background()
	ids, err := reader.ListTesteeIDsInStores(ctx, 1, []uint64{7}, false)
	require.NoError(t, err)
	require.Equal(t, []uint64{1}, ids)
	require.NoError(t, db.Exec("UPDATE testee SET store_id=8 WHERE id=1").Error)
	ids, err = reader.ListTesteeIDsInStores(ctx, 1, []uint64{7}, false)
	require.NoError(t, err)
	require.Empty(t, ids)
	ids, err = reader.ListTesteeIDsInStores(ctx, 1, nil, true)
	require.NoError(t, err)
	require.Equal(t, []uint64{1, 2}, ids)
	ids, err = reader.ListTesteeIDsInStores(ctx, 1, nil, false)
	require.NoError(t, err)
	require.Empty(t, ids)
	_, err = reader.ListTesteeIDsInStores(ctx, 0, nil, true)
	require.Error(t, err)
}

func TestProfileFilterIntersectsCurrentStoreBeforeCount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE testee(id INTEGER,org_id INTEGER,profile_id INTEGER,store_id INTEGER,deleted_at DATETIME)").Error)
	require.NoError(t, db.Exec("INSERT INTO testee VALUES(1,1,11,7,NULL),(2,1,22,8,NULL),(3,2,11,7,NULL)").Error)
	reader := NewReadModel(db).(*readModel)
	id := uint64(22)
	filter := rm.TesteeFilter{OrgID: 1, ProfileID: &id, RestrictToStoreScope: true, AllowedStoreIDs: []uint64{7}}
	var count int64
	require.NoError(t, reader.applyTesteeFilter(db.Table("testee"), filter).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Exec("UPDATE testee SET store_id=7 WHERE id=2").Error)
	require.NoError(t, reader.applyTesteeFilter(db.Table("testee"), filter).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestRelationScopeFiltersBeforePaginationAndFollowsTransfer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE testee(id INTEGER PRIMARY KEY,org_id INTEGER,store_id INTEGER,deleted_at DATETIME)").Error)
	require.NoError(t, db.Exec("CREATE TABLE clinician_relation(id INTEGER PRIMARY KEY,org_id INTEGER,clinician_id INTEGER,testee_id INTEGER,is_active BOOLEAN,bound_at DATETIME,deleted_at DATETIME)").Error)
	require.NoError(t, db.Exec("INSERT INTO testee VALUES(1,1,7,NULL),(2,1,8,NULL),(3,1,NULL,NULL),(4,2,7,NULL),(5,1,7,'2026-01-01'),(6,1,7,NULL)").Error)
	for id := 1; id <= 6; id++ {
		require.NoError(t, db.Exec("INSERT INTO clinician_relation VALUES(?,1,10,?,1,'2026-01-01',NULL)", id, id).Error)
	}
	reader := NewReadModel(db).(*readModel)
	filter := rm.RelationFilter{OrgID: 1, ClinicianID: 10, ActiveOnly: true, RestrictToStoreScope: true, AllowedStoreIDs: []uint64{7}, Limit: 1, Offset: 1}
	rows, total, err := reader.listRelationPOs(context.Background(), filter, true)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, rows, 1)
	require.EqualValues(t, 1, rows[0].TesteeID)
	require.NoError(t, db.Exec("UPDATE testee SET store_id=8 WHERE id=1").Error)
	rows, total, err = reader.listRelationPOs(context.Background(), filter, true)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Empty(t, rows)
	filter.Offset = 0
	filter.AllowedStoreIDs = nil
	rows, total, err = reader.listRelationPOs(context.Background(), filter, true)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows)
	filter.AllAssignedStores = true
	filter.Limit = 0
	rows, total, err = reader.listRelationPOs(context.Background(), filter, true)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Len(t, rows, 3)
	filter.TesteeIDs = []uint64{3, 4, 5}
	rows, total, err = reader.listRelationPOs(context.Background(), filter, true)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows)
}

func TestScopedRelationPageRejectsTransferDuringHydration(t *testing.T) {
	for _, kind := range []string{"assigned", "relations"} {
		t.Run(kind, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.Exec("CREATE TABLE testee(id INTEGER PRIMARY KEY,org_id INTEGER,store_id INTEGER,deleted_at DATETIME)").Error)
			require.NoError(t, db.Exec("CREATE TABLE clinician_relation(id INTEGER PRIMARY KEY,org_id INTEGER,clinician_id INTEGER,testee_id INTEGER,is_active BOOLEAN,bound_at DATETIME,deleted_at DATETIME)").Error)
			require.NoError(t, db.Exec("INSERT INTO testee VALUES(1,1,7,NULL)").Error)
			require.NoError(t, db.Exec("INSERT INTO clinician_relation VALUES(1,1,10,1,1,'2026-01-01',NULL)").Error)
			transferred := false
			require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:transfer_before_hydration", func(tx *gorm.DB) {
				if _, ok := tx.Statement.Dest.(*[]*ClinicianRelationPO); ok && !transferred {
					transferred = true
					require.NoError(t, db.Exec("UPDATE testee SET store_id=8 WHERE id=1").Error)
				}
			}))
			reader := NewReadModel(db)
			filter := rm.RelationFilter{OrgID: 1, ClinicianID: 10, ActiveOnly: true, RestrictToStoreScope: true, AllowedStoreIDs: []uint64{7}, Limit: 10}
			if kind == "assigned" {
				rows, total, err := reader.ListAssignedTestees(context.Background(), filter)
				require.True(t, cbErrors.IsCode(err, code.ErrConflict), "expected ownership conflict: %v", err)
				require.Nil(t, rows)
				require.Zero(t, total)
			} else {
				rows, total, err := reader.ListClinicianRelations(context.Background(), filter)
				require.True(t, cbErrors.IsCode(err, code.ErrConflict), "expected ownership conflict: %v", err)
				require.Nil(t, rows)
				require.Zero(t, total)
			}
			require.True(t, transferred, "test must move ownership between actual queries")
			filter.AllowedStoreIDs = []uint64{8}
			rows, total, err := reader.ListAssignedTestees(context.Background(), filter)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.EqualValues(t, 1, total)
			require.EqualValues(t, 8, *rows[0].StoreID)

		})
	}
}
