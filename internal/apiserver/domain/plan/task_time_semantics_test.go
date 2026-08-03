package plan

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

func TestAssessmentTaskDueAndRescheduleUseCalendarDays(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	planned := time.Date(2026, 3, 1, 19, 0, 0, 0, loc)
	task := NewAssessmentTask(NewAssessmentPlanID(), 1, 1, testee.NewID(1), "scale", planned)
	if want := planned.AddDate(0, 0, 7); !task.GetDueAt().Equal(want) {
		t.Fatalf("due_at=%s want=%s", task.GetDueAt(), want)
	}

	lifecycle := NewTaskLifecycle()
	openAt := planned
	if err := lifecycle.OpenAt(t.Context(), task, "token", "url", openAt, TaskEntryExpiresAt(openAt)); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.ExpireManually(task, openAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	rescheduled := planned.AddDate(0, 1, 0)
	if err := lifecycle.Reschedule(t.Context(), task, rescheduled); err != nil {
		t.Fatal(err)
	}
	if !task.GetDueAt().Equal(rescheduled.AddDate(0, 0, 7)) || task.GetOpenAt() != nil || task.GetExpireAt() != nil || task.GetExpirationReason() != "" {
		t.Fatalf("reschedule did not reset time semantics: due=%s open=%v expire=%v reason=%q", task.GetDueAt(), task.GetOpenAt(), task.GetExpireAt(), task.GetExpirationReason())
	}
}

func TestTaskDueAtConvertsUTCInputToShanghai(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	plannedAt := time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC)
	want := time.Date(2026, 8, 8, 19, 0, 0, 0, loc)
	got := TaskDueAt(plannedAt)
	if !got.Equal(want) || got.Location().String() != "Asia/Shanghai" {
		t.Fatalf("TaskDueAt() = %v (%s), want %v (Asia/Shanghai)", got, got.Location(), want)
	}
}

func TestTaskCompletionAllowsOverdueBeforeEntryExpiry(t *testing.T) {
	planned := time.Date(2026, 8, 1, 8, 0, 0, 0, time.Local)
	task := NewAssessmentTask(NewAssessmentPlanID(), 1, 1, testee.NewID(1), "scale", planned)
	lifecycle := NewTaskLifecycle()
	openAt := planned.Add(23 * time.Hour)
	expireAt := TaskEntryExpiresAt(openAt)
	if err := lifecycle.OpenAt(t.Context(), task, "token", "url", openAt, expireAt); err != nil {
		t.Fatal(err)
	}
	completedAt := planned.AddDate(0, 0, 7).Add(time.Hour)
	if !completedAt.After(task.GetDueAt()) {
		t.Fatal("test setup requires overdue completion")
	}
	if err := lifecycle.CompleteAt(t.Context(), task, assessment.ID(9), completedAt); err != nil {
		t.Fatalf("overdue completion inside entry validity should succeed: %v", err)
	}
}

func TestTaskCompletionRejectsEntryExpiryBoundary(t *testing.T) {
	planned := time.Now()
	task := NewAssessmentTask(NewAssessmentPlanID(), 1, 1, testee.NewID(1), "scale", planned)
	lifecycle := NewTaskLifecycle()
	expireAt := TaskEntryExpiresAt(planned)
	if err := lifecycle.OpenAt(t.Context(), task, "token", "url", planned, expireAt); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.CompleteAt(t.Context(), task, assessment.ID(9), expireAt); err == nil {
		t.Fatal("completion at expire_at must fail")
	}
}

func TestTaskMissedOpenWindowTransition(t *testing.T) {
	planned := time.Now().Add(-48 * time.Hour)
	task := NewAssessmentTask(NewAssessmentPlanID(), 1, 1, testee.NewID(1), "scale", planned)
	lifecycle := NewTaskLifecycle()
	expiredAt := TaskOpenWindowEndsAt(planned)
	if err := lifecycle.ExpireMissedOpenWindow(task, expiredAt); err != nil {
		t.Fatal(err)
	}
	if !task.IsExpired() || task.GetExpirationReason() != TaskExpirationReasonMissedOpenWindow || task.GetExpiredAt() == nil || !task.GetExpiredAt().Equal(expiredAt) {
		t.Fatalf("unexpected missed transition: status=%s reason=%s expired_at=%v", task.GetStatus(), task.GetExpirationReason(), task.GetExpiredAt())
	}
}

func TestTaskOpenWindowBoundaries(t *testing.T) {
	planned := time.Now().Add(time.Hour)
	lifecycle := NewTaskLifecycle()

	tooEarly := NewAssessmentTask(NewAssessmentPlanID(), 1, 1, testee.NewID(1), "scale", planned)
	if err := lifecycle.OpenAt(t.Context(), tooEarly, "token", "url", planned.Add(-time.Nanosecond), TaskEntryExpiresAt(planned.Add(-time.Nanosecond))); err == nil {
		t.Fatal("opening before planned_at must fail")
	}

	atBoundary := NewAssessmentTask(NewAssessmentPlanID(), 2, 1, testee.NewID(1), "scale", planned)
	windowEnd := TaskOpenWindowEndsAt(planned)
	if err := lifecycle.OpenAt(t.Context(), atBoundary, "token", "url", windowEnd, TaskEntryExpiresAt(windowEnd)); err == nil {
		t.Fatal("opening at planned_at+24h must fail")
	}

	inside := NewAssessmentTask(NewAssessmentPlanID(), 3, 1, testee.NewID(1), "scale", planned)
	openAt := windowEnd.Add(-time.Nanosecond)
	if err := lifecycle.OpenAt(t.Context(), inside, "token", "url", openAt, TaskEntryExpiresAt(openAt)); err != nil {
		t.Fatalf("opening immediately before window end should succeed: %v", err)
	}
}

func TestTaskLifecycleRejectsIllegalExpirationTransitions(t *testing.T) {
	planned := time.Now().Add(-time.Hour)
	lifecycle := NewTaskLifecycle()
	pending := NewAssessmentTask(NewAssessmentPlanID(), 1, 1, testee.NewID(1), "scale", planned)
	if err := lifecycle.ExpireAt(pending, TaskExpirationReasonEntryTimeout, time.Now()); err == nil {
		t.Fatal("pending task cannot expire as entry timeout")
	}
	if err := lifecycle.ExpireMissedOpenWindow(pending, planned.Add(time.Hour)); err == nil {
		t.Fatal("pending task cannot expire before its opening window ends")
	}

	opened := NewAssessmentTask(NewAssessmentPlanID(), 2, 1, testee.NewID(1), "scale", planned)
	openAt := planned
	if err := lifecycle.OpenAt(t.Context(), opened, "token", "url", openAt, TaskEntryExpiresAt(openAt)); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.ExpireMissedOpenWindow(opened, TaskOpenWindowEndsAt(planned)); err == nil {
		t.Fatal("opened task cannot expire as missed opening window")
	}
	if err := lifecycle.ExpireAt(opened, TaskExpirationReasonEntryTimeout, TaskEntryExpiresAt(openAt).Add(-time.Nanosecond)); err == nil {
		t.Fatal("entry timeout before expire_at must fail")
	}
}
