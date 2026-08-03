package plan

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestSaveRescheduledUsesExpectedRevisionCASAndWritesClearedFields(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	repository := NewTaskRepository(db).(*taskRepository)
	plannedAt := time.Date(2026, 8, 5, 19, 0, 0, 0, time.UTC)
	task := domainPlan.NewAssessmentTaskAt(domainPlan.NewAssessmentPlanID(), 1, 7, domainTestee.NewID(99), "scale", plannedAt, plannedAt)
	task.AssignEnrollment(domainPlan.PlanEnrollmentID(123))
	if err := domainPlan.NewTaskLifecycle().RescheduleAt(context.Background(), task, plannedAt.AddDate(0, 0, 3), plannedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("(?s)UPDATE `assessment_task` SET .*`assessment_id`=\\?.*`canceled_at`=\\?.*`completed_at`=\\?.*`entry_token`=\\?.*`expired_at`=\\?.*`open_at`=\\?.*`schedule_revision`=\\?.*`status`=\\?.* WHERE id = \\? AND org_id = \\? AND schedule_revision = \\? AND deleted_at IS NULL").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repository.SaveRescheduled(context.Background(), task, 1); err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("(?s)UPDATE `assessment_task` SET .* WHERE id = \\? AND org_id = \\? AND schedule_revision = \\? AND deleted_at IS NULL").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	err = repository.SaveRescheduled(context.Background(), task, 1)
	if err == nil || !baseerrors.IsCode(err, code.ErrConflict) {
		t.Fatalf("err=%v want revision conflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
