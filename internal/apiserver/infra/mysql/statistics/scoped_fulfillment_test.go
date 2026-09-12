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

func TestScopedFulfillmentKeepsLatestScheduleAndCancellation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE testee(id INTEGER,org_id INTEGER,store_id INTEGER,deleted_at DATETIME)").Error)
	require.NoError(t, db.Exec("INSERT INTO testee VALUES(1,1,7,NULL),(2,1,8,NULL)").Error)
	require.NoError(t, db.Exec(`CREATE TABLE statistics_plan_fact(id INTEGER PRIMARY KEY AUTOINCREMENT,org_id INTEGER,plan_id INTEGER,testee_id INTEGER,task_id INTEGER,fact_type TEXT,schedule_revision INTEGER,schedule_planned_at DATETIME,schedule_due_at DATETIME,planned_at DATETIME,due_at DATETIME,completed_at DATETIME,task_status TEXT)`).Error)
	date := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	due := date.Add(12 * time.Hour)
	insert := func(subject, task, revision int, kind, status string, complete any) {
		require.NoError(t, db.Table("statistics_plan_fact").Create(map[string]any{"org_id": 1, "plan_id": 1, "testee_id": subject, "task_id": task, "fact_type": kind, "schedule_revision": revision, "schedule_planned_at": date, "schedule_due_at": due, "planned_at": date, "due_at": due, "completed_at": complete, "task_status": status}).Error)
	}
	insert(1, 10, 1, "task_schedule_defined", "", nil)
	insert(1, 10, 1, "task_schedule_terminal", "completed", date.Add(10*time.Hour))
	insert(1, 10, 2, "task_schedule_defined", "", nil) // supersedes old completion
	insert(1, 11, 1, "task_schedule_defined", "", nil)
	insert(1, 11, 1, "task_schedule_terminal", "canceled", nil)
	insert(1, 12, 0, "task_created", "", nil) // legacy task remains supported
	insert(1, 12, 0, "task_completed", "completed", due.Add(time.Hour))
	insert(2, 20, 1, "task_schedule_defined", "", nil)
	store := NewReadStore(db, nil)
	rows, err := store.scopedFulfillment(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, date, date.AddDate(0, 0, 1), date.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 2, rows[0].Planned)
	require.EqualValues(t, 2, rows[0].Due)
	require.Zero(t, rows[0].CompletedOnTime)
	require.EqualValues(t, 1, rows[0].CompletedOverdue)
	require.EqualValues(t, 1, rows[0].UncompletedOverdue)
	require.NoError(t, db.Exec("UPDATE testee SET store_id=8 WHERE id=1").Error)
	rows, err = store.scopedFulfillment(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, date, date.AddDate(0, 0, 1), date.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Empty(t, rows)
	rows, err = store.scopedFulfillment(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{8}}, date, date.AddDate(0, 0, 1), date.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 3, rows[0].Planned)
}
