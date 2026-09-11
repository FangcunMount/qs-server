package operatorretirement

import (
	"testing"
	"time"
)

func TestTaskRequiresObservedRevocationAndRejectsInvalidTransitions(t *testing.T) {
	now := time.Now()
	task, err := New(1, 2, 3, 4, 1, "request", "reason", now)
	if err != nil {
		t.Fatal(err)
	}
	if task.Complete(10, false, now) == nil {
		t.Fatal("completed before revocation")
	}
	if err = task.MarkRevoked(10, now); err != nil {
		t.Fatal(err)
	}
	if task.Complete(9, false, now) == nil || task.Complete(10, true, now) == nil {
		t.Fatal("unconverged authorization accepted")
	}
	if err = task.Complete(10, false, now); err != nil {
		t.Fatal(err)
	}
	if task.MarkRevoked(11, now) == nil {
		t.Fatal("completed task regressed")
	}
}
