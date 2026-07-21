package plan

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

func TestEnrollmentLifecycleSupportsCloseAndSecondRound(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	first := NewEnrollment(9, NewAssessmentPlanID(), testee.NewID(1001), 1, start, start.Add(time.Hour))
	closedAt := start.AddDate(0, 0, 14)
	first.Close(closedAt)

	if first.Status() != EnrollmentStatusClosed || first.ClosedAt() == nil || !first.ClosedAt().Equal(closedAt) {
		t.Fatalf("unexpected closed enrollment: status=%s closed_at=%v", first.Status(), first.ClosedAt())
	}
	second := NewEnrollment(first.OrgID(), first.PlanID(), first.TesteeID(), first.Round()+1, start.AddDate(0, 1, 0), closedAt.Add(time.Hour))
	if second.Round() != 2 || !second.IsActive() {
		t.Fatalf("expected active round 2, got round=%d status=%s", second.Round(), second.Status())
	}
}

func TestEnrollmentTerminationIsIdempotent(t *testing.T) {
	now := time.Now()
	enrollment := NewEnrollment(9, NewAssessmentPlanID(), testee.NewID(1002), 1, now, now)
	enrollment.Terminate(now.Add(time.Hour), " user requested ")
	enrollment.Terminate(now.Add(2*time.Hour), "must not overwrite")

	if enrollment.Status() != EnrollmentStatusTerminated {
		t.Fatalf("expected terminated status, got %s", enrollment.Status())
	}
	if enrollment.TerminatedReason() != "user requested" {
		t.Fatalf("unexpected termination reason %q", enrollment.TerminatedReason())
	}
}

func TestAssessmentTaskRecordsTerminalTransitionTimes(t *testing.T) {
	now := time.Now()
	task := NewAssessmentTask(NewAssessmentPlanID(), 1, 9, testee.NewID(1003), "S-1", now)
	lifecycle := NewTaskLifecycle()
	if err := lifecycle.Open(t.Context(), task, "token", "https://example.test", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Expire(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if task.GetExpiredAt() == nil || task.GetCanceledAt() != nil {
		t.Fatalf("expected only expired_at, got expired_at=%v canceled_at=%v", task.GetExpiredAt(), task.GetCanceledAt())
	}
}
