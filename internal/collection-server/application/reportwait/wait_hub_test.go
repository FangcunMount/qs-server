package reportwait

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

func TestWaitHubNotifyAndUnregister(t *testing.T) {
	hub := NewInMemoryWaitHub()
	ch, cancel := hub.Register("90001")
	defer cancel()

	signal := reportstatus.ChangedSignal{
		AssessmentID: "90001",
		Status:       "completed",
		OccurredAt:   time.Now().UTC(),
	}
	hub.Notify(signal)

	select {
	case got := <-ch:
		if got.AssessmentID != "90001" || got.Status != "completed" {
			t.Fatalf("unexpected signal: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for signal")
	}

	cancel()
	hub.Notify(signal)
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel after unregister")
		}
	default:
	}
}

func TestWaitHubIsolationByAssessmentID(t *testing.T) {
	hub := NewInMemoryWaitHub()
	ch1, cancel1 := hub.Register("1")
	defer cancel1()
	ch2, cancel2 := hub.Register("2")
	defer cancel2()

	hub.Notify(reportstatus.ChangedSignal{AssessmentID: "1", Status: "completed", OccurredAt: time.Now().UTC()})

	select {
	case <-ch1:
	case <-time.After(time.Second):
		t.Fatal("assessment 1 waiter should receive signal")
	}
	select {
	case <-ch2:
		t.Fatal("assessment 2 waiter should not receive signal for assessment 1")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestWaitHubActiveWaiters(t *testing.T) {
	hub := NewInMemoryWaitHub()
	if hub.ActiveWaiters() != 0 {
		t.Fatalf("expected 0 active waiters, got %d", hub.ActiveWaiters())
	}
	_, cancel := hub.Register("42")
	if hub.ActiveWaiters() != 1 {
		t.Fatalf("expected 1 active waiter, got %d", hub.ActiveWaiters())
	}
	cancel()
	if hub.ActiveWaiters() != 0 {
		t.Fatalf("expected 0 active waiters after cancel, got %d", hub.ActiveWaiters())
	}
}
