package statistics

import (
	"context"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

func TestProjectionRegistriesSeparateWindowRepairFromGlobalPublication(t *testing.T) {
	daily := NewDailyProjections(nil)
	global := NewGlobalProjections(nil)
	if got := projectionNames(daily); !reflect.DeepEqual(got, []string{"access_daily", "assessment_daily", "plan_activity_daily"}) {
		t.Fatalf("daily=%v", got)
	}
	if got := projectionNames(global); !reflect.DeepEqual(got, []string{"plan_fulfillment", "organization_snapshot"}) {
		t.Fatalf("global=%v", got)
	}
}

func TestFulfillmentNumericContract(t *testing.T) {
	due := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	onTime, overdue := due, due.Add(time.Nanosecond)
	got := calculateFulfillmentContract([]fulfillmentContractTask{
		{TesteeID: 1, DueAt: due, CompletedAt: &onTime},
		{TesteeID: 1, DueAt: due, CompletedAt: &overdue},
		{TesteeID: 2, DueAt: due},
		{TesteeID: 3, DueAt: due, Canceled: true},
	}, due.Add(24*time.Hour))
	want := fulfillmentContractCounts{PlannedTasks: 3, PlannedParticipants: 2, DueTasks: 3, CompletedOnTime: 1, CompletedOverdue: 1, UncompletedOverdue: 1}
	if got != want {
		t.Fatalf("fulfillment=%+v want=%+v", got, want)
	}
}

func TestFulfillmentProjectionSelectsLatestScheduleRevisionAndKeepsLegacyFallback(t *testing.T) {
	writer, mock := newFactWriterTestDB(t)
	cutoff := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM statistics_plan_fulfillment_daily WHERE org_id=?")).
		WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("(?s)INSERT INTO statistics_plan_fulfillment_daily.*ROW_NUMBER\\(\\) OVER \\(PARTITION BY org_id,task_id ORDER BY schedule_revision DESC,id DESC\\).*task_schedule_terminal.*task_due_defined.*"+regexp.QuoteMeta("SUM(CASE WHEN completed_at>due_at THEN 1 ELSE 0 END)")).
		WithArgs(int64(1), int64(1), cutoff).WillReturnResult(sqlmock.NewResult(0, 5))

	result, err := (&PlanFulfillmentProjection{db: writer.db}).Project(context.Background(), statisticsDomain.ProjectionRequest{OrgID: 1, CutoffAt: cutoff})
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows != 5 {
		t.Fatalf("rows=%d", result.Rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func projectionNames(items []statisticsDomain.Projection) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name())
	}
	return names
}
