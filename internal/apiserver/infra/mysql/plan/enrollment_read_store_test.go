package plan

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestEnrollmentReadStoreListsEnrollmentsAndTasks(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(mysqlDriver.New(mysqlDriver.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	joinedAt := time.Date(2025, 1, 1, 8, 8, 0, 0, time.UTC)
	plannedAt := joinedAt.Add(time.Minute)
	completedAt := plannedAt.Add(20 * time.Minute)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `plan_enrollment` WHERE org_id=? AND testee_id=? AND deleted_at IS NULL")).
		WithArgs(int64(1), uint64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("(?s)"+regexp.QuoteMeta("SELECT * FROM `plan_enrollment` WHERE org_id=? AND testee_id=? AND deleted_at IS NULL")+".*"+regexp.QuoteMeta("ORDER BY joined_at DESC,id DESC")+".*").
		WithArgs(int64(1), uint64(42), 100).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "org_id", "plan_id", "testee_id", "round", "start_date", "status", "joined_at",
			"closed_at", "terminated_at", "terminated_reason", "record_origin",
		}).AddRow(
			uint64(1001), int64(1), uint64(2001), uint64(42), uint32(1), joinedAt, "active", joinedAt,
			nil, nil, "", "native",
		))
	mock.ExpectQuery("(?s)" + regexp.QuoteMeta("SELECT id,enrollment_id,seq,scale_code,status,planned_at,open_at,expire_at,completed_at,expired_at,canceled_at,assessment_id FROM `assessment_task`") + ".*").
		WithArgs(uint64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "enrollment_id", "seq", "scale_code", "status", "planned_at", "open_at", "expire_at",
			"completed_at", "expired_at", "canceled_at", "assessment_id",
		}).AddRow(
			uint64(3001), uint64(1001), 1, "SDS", "completed", plannedAt, plannedAt, nil,
			completedAt, nil, nil, uint64(4001),
		))

	items, total, err := NewEnrollmentReadStore(db, nil).ListEnrollments(context.Background(), planapp.EnrollmentQuery{
		OrgID: 1, TesteeID: 42, Page: 1, PageSize: 100,
	})
	if err != nil {
		t.Fatalf("list enrollments: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("total/items = %d/%d, want 1/1", total, len(items))
	}
	item := items[0]
	if item.ID != 1001 || item.PlanID != 2001 || item.TesteeID != 42 || item.RecordOrigin != "native" {
		t.Fatalf("unexpected enrollment item: %#v", item)
	}
	if len(item.Tasks) != 1 {
		t.Fatalf("tasks = %#v, want one task", item.Tasks)
	}
	task := item.Tasks[0]
	if task.ID != 3001 || task.ScaleCode != "SDS" || task.AssessmentID == nil || *task.AssessmentID != "4001" {
		t.Fatalf("unexpected enrollment task: %#v", task)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
