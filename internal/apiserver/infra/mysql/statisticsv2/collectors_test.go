package statisticsv2

import (
	"testing"
	"time"
)

func TestTaskFactDoesNotBackfillLaterLifecycleIntoEarlierEvent(t *testing.T) {
	created := map[string]any{}
	applyTaskLifecycleFields(created, "task_created", nil)
	if _, exists := created["task_status"]; exists {
		t.Fatal("task_created must not copy a later current status")
	}
	if _, exists := created["completed_at"]; exists {
		t.Fatal("task_created must not copy a later completion time")
	}

	completedAt := time.Date(2026, 7, 20, 9, 30, 0, 0, time.FixedZone("CST", 8*3600))
	completed := map[string]any{}
	applyTaskLifecycleFields(completed, "task_completed", &completedAt)
	if completed["task_status"] != "completed" || completed["completed_at"] != &completedAt {
		t.Fatalf("completed fact=%v", completed)
	}
}
