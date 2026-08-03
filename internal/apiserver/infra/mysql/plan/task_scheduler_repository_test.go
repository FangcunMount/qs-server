package plan

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestTaskSchedulerRepositoryQueriesAreOrganizationScopedAndKeysetPaged(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	repository, ok := NewTaskRepository(db).(domainPlan.AssessmentTaskSchedulerRepository)
	if !ok {
		t.Fatal("task repository must implement bounded scheduler scans")
	}

	from := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
	through := from.Add(24 * time.Hour)
	cursor := from.Add(time.Hour)
	emptyRows := func() *sqlmock.Rows { return sqlmock.NewRows([]string{"id"}) }

	mock.ExpectQuery("(?s)"+regexp.QuoteMeta("FROM `assessment_task` WHERE (org_id = ? AND status = ? AND planned_at > ? AND planned_at <= ? AND deleted_at IS NULL) AND ((planned_at > ?) OR (planned_at = ? AND id > ?)) ORDER BY planned_at ASC,id ASC LIMIT ?")).
		WithArgs(int64(7), "pending", from, through, cursor, cursor, uint64(99), 200).
		WillReturnRows(emptyRows())
	if _, err := repository.FindOpenEligibleTaskPage(context.Background(), 7, from, through, cursor, 99, 200); err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery("(?s)"+regexp.QuoteMeta("FROM `assessment_task` WHERE (org_id = ? AND status = ? AND planned_at <= ? AND deleted_at IS NULL) AND ((planned_at > ?) OR (planned_at = ? AND id > ?)) ORDER BY planned_at ASC,id ASC LIMIT ?")).
		WithArgs(int64(7), "pending", through, cursor, cursor, uint64(99), 200).
		WillReturnRows(emptyRows())
	if _, err := repository.FindMissedPendingTaskPage(context.Background(), 7, through, cursor, 99, 200); err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery("(?s)"+regexp.QuoteMeta("FROM `assessment_task` WHERE (org_id = ? AND status = ? AND expire_at IS NOT NULL AND expire_at <= ? AND deleted_at IS NULL) AND ((expire_at > ?) OR (expire_at = ? AND id > ?)) ORDER BY expire_at ASC,id ASC LIMIT ?")).
		WithArgs(int64(7), "opened", through, cursor, cursor, uint64(99), 200).
		WillReturnRows(emptyRows())
	if _, err := repository.FindEntryExpiredTaskPage(context.Background(), 7, through, cursor, 99, 200); err != nil {
		t.Fatal(err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
