package statistics

import (
	"reflect"
	"testing"
	"time"

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

func projectionNames(items []statisticsDomain.Projection) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name())
	}
	return names
}
