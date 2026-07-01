package reportnotify

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

func TestNotifierNotifyAndUnsubscribe(t *testing.T) {
	notifier := NewInMemoryNotifier()
	ch, cancel := notifier.Subscribe("90001")
	defer cancel()

	signal := reportstatus.ChangedSignal{
		AssessmentID: "90001",
		Status:       "completed",
		OccurredAt:   time.Now().UTC(),
	}
	notifier.Notify(signal)

	select {
	case got := <-ch:
		if got.AssessmentID != "90001" || got.Status != "completed" {
			t.Fatalf("unexpected signal: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for signal")
	}

	cancel()
	notifier.Notify(signal)
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel after unsubscribe")
		}
	default:
	}
}

func TestNotifierIsolationByAssessmentID(t *testing.T) {
	notifier := NewInMemoryNotifier()
	ch1, cancel1 := notifier.Subscribe("1")
	defer cancel1()
	ch2, cancel2 := notifier.Subscribe("2")
	defer cancel2()

	notifier.Notify(reportstatus.ChangedSignal{AssessmentID: "1", Status: "completed", OccurredAt: time.Now().UTC()})

	select {
	case <-ch1:
	case <-time.After(time.Second):
		t.Fatal("assessment 1 subscriber should receive signal")
	}
	select {
	case <-ch2:
		t.Fatal("assessment 2 subscriber should not receive signal for assessment 1")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNotifierActiveSubscriptions(t *testing.T) {
	notifier := NewInMemoryNotifier()
	if notifier.ActiveSubscriptions() != 0 {
		t.Fatalf("expected 0 active subscriptions, got %d", notifier.ActiveSubscriptions())
	}
	_, cancel := notifier.Subscribe("42")
	if notifier.ActiveSubscriptions() != 1 {
		t.Fatalf("expected 1 active subscription, got %d", notifier.ActiveSubscriptions())
	}
	cancel()
	if notifier.ActiveSubscriptions() != 0 {
		t.Fatalf("expected 0 active subscriptions after cancel, got %d", notifier.ActiveSubscriptions())
	}
}
