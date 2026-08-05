//go:build integration

package statistics

import (
	"context"
	"os"
	"testing"
	"time"

	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestFulfillmentProjectionExecutesLatestRevisionMatrixAgainstMySQL(t *testing.T) {
	dsn := os.Getenv("QS_STATISTICS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("QS_STATISTICS_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var database string
	if err := db.Raw("SELECT DATABASE()").Scan(&database).Error; err != nil {
		t.Fatal(err)
	}
	if database != "qs_statistics_test" {
		t.Fatalf("refusing integration test against database %q", database)
	}
	for _, table := range []string{"statistics_plan_fulfillment_daily", "statistics_plan_fact"} {
		if err := db.Exec("DROP TABLE IF EXISTS " + table).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = db.Exec("DROP TABLE IF EXISTS statistics_plan_fulfillment_daily").Error
		_ = db.Exec("DROP TABLE IF EXISTS statistics_plan_fact").Error
	})
	if err := db.Exec(`CREATE TABLE statistics_plan_fact (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		org_id BIGINT NOT NULL, plan_id BIGINT UNSIGNED, testee_id BIGINT UNSIGNED, task_id BIGINT UNSIGNED,
		fact_type VARCHAR(64) NOT NULL, planned_at DATETIME(3), due_at DATETIME(3), completed_at DATETIME(3), task_status VARCHAR(32),
		schedule_revision INT UNSIGNED, schedule_planned_at DATETIME(3), schedule_due_at DATETIME(3)
	) ENGINE=InnoDB`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE statistics_plan_fulfillment_daily (
		org_id BIGINT NOT NULL, cohort_date DATE NOT NULL, plan_id BIGINT UNSIGNED NOT NULL,
		planned_task_count BIGINT NOT NULL, planned_participant_count BIGINT NOT NULL, due_task_count BIGINT NOT NULL,
		completed_on_time_count BIGINT NOT NULL, completed_overdue_count BIGINT NOT NULL, uncompleted_overdue_count BIGINT NOT NULL,
		PRIMARY KEY (org_id,cohort_date,plan_id)
	) ENGINE=InnoDB`).Error; err != nil {
		t.Fatal(err)
	}

	insert := func(values ...any) {
		t.Helper()
		if err := db.Exec(`INSERT INTO statistics_plan_fact
			(org_id,plan_id,testee_id,task_id,fact_type,planned_at,due_at,completed_at,task_status,schedule_revision,schedule_planned_at,schedule_due_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, values...).Error; err != nil {
			t.Fatal(err)
		}
	}
	date := func(day int) time.Time { return time.Date(2026, 8, day, 19, 0, 0, 0, statisticsDomain.Shanghai) }

	// Task 1 was canceled in the legacy round. Revision 2 moves both cohorts to
	// earlier dates and completes overdue; the old cancellation must not exclude it.
	insert(1, 10, 101, 1, "task_created", date(1), date(20), nil, nil, nil, nil, nil)
	insert(1, 10, 101, 1, "task_canceled", date(1), date(20), nil, "canceled", nil, nil, nil)
	insert(1, 10, 101, 1, "task_schedule_defined", nil, nil, nil, nil, 1, date(1), date(20))
	insert(1, 10, 101, 1, "task_schedule_defined", nil, nil, nil, nil, 2, date(2), date(10))
	insert(1, 10, 101, 1, "task_schedule_terminal", nil, nil, date(11), "completed", 2, nil, nil)

	// Task 2 is canceled in its latest revision and is fully excluded.
	insert(1, 10, 102, 2, "task_created", date(1), date(8), nil, nil, nil, nil, nil)
	insert(1, 10, 102, 2, "task_schedule_defined", nil, nil, nil, nil, 2, date(3), date(10))
	insert(1, 10, 102, 2, "task_schedule_terminal", nil, nil, nil, "canceled", 2, nil, nil)

	// Task 3 proves task_due_defined remains a valid legacy fallback.
	insert(1, 10, 103, 3, "task_created", date(4), nil, nil, nil, nil, nil, nil)
	insert(1, 10, 103, 3, "task_due_defined", date(4), date(12), nil, nil, nil, nil, nil)
	insert(1, 10, 103, 3, "task_completed", date(4), nil, date(12), "completed", nil, nil, nil)

	// Task 4 has no terminal in its latest revision and is overdue at cutoff.
	insert(1, 10, 104, 4, "task_created", date(5), nil, nil, nil, nil, nil, nil)
	insert(1, 10, 104, 4, "task_schedule_defined", nil, nil, nil, nil, 1, date(5), date(13))

	// Task 5 proves that the revision-scoped schedule Fact is self-contained;
	// fulfillment must not depend on a separately collected task_created Fact.
	insert(1, 10, 105, 5, "task_schedule_defined", nil, nil, nil, nil, 1, date(6), date(14))

	cutoff := date(15)
	if _, err := (&PlanFulfillmentProjection{db: db}).Project(context.Background(), statisticsDomain.ProjectionRequest{OrgID: 1, CutoffAt: cutoff}); err != nil {
		t.Fatal(err)
	}
	var totals fulfillmentContractCounts
	if err := db.Raw(`SELECT COALESCE(SUM(planned_task_count),0),COALESCE(SUM(planned_participant_count),0),COALESCE(SUM(due_task_count),0),COALESCE(SUM(completed_on_time_count),0),COALESCE(SUM(completed_overdue_count),0),COALESCE(SUM(uncompleted_overdue_count),0) FROM statistics_plan_fulfillment_daily WHERE org_id=1`).
		Row().Scan(&totals.PlannedTasks, &totals.PlannedParticipants, &totals.DueTasks, &totals.CompletedOnTime, &totals.CompletedOverdue, &totals.UncompletedOverdue); err != nil {
		t.Fatal(err)
	}
	want := fulfillmentContractCounts{PlannedTasks: 4, PlannedParticipants: 4, DueTasks: 4, CompletedOnTime: 1, CompletedOverdue: 1, UncompletedOverdue: 2}
	if totals != want {
		t.Fatalf("totals=%+v want=%+v", totals, want)
	}
	var completedOverdue, uncompletedOverdue int
	if err := db.Raw(`SELECT completed_overdue_count,uncompleted_overdue_count FROM statistics_plan_fulfillment_daily WHERE org_id=1 AND plan_id=10 AND cohort_date='2026-08-14'`).
		Row().Scan(&completedOverdue, &uncompletedOverdue); err != nil {
		t.Fatal(err)
	}
	if completedOverdue != 0 || uncompletedOverdue != 1 {
		t.Fatalf("unfinished-only due cohort completed_overdue=%d uncompleted_overdue=%d want 0,1", completedOverdue, uncompletedOverdue)
	}
	var earlierDueCount int
	if err := db.Raw(`SELECT due_task_count FROM statistics_plan_fulfillment_daily WHERE org_id=1 AND plan_id=10 AND cohort_date='2026-08-10'`).Scan(&earlierDueCount).Error; err != nil {
		t.Fatal(err)
	}
	if earlierDueCount != 1 {
		t.Fatalf("latest revision due cohort count=%d want=1", earlierDueCount)
	}
}
