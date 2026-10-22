package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing/retrydispatch"
	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
)

func TestRetryDispatchRunnerPausesAndJoinsRealWork(t *testing.T) {
	var ready atomic.Bool
	var scans atomic.Int32
	checked := make(chan struct{}, 1)
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	limits := opts.RetryDispatchOptions{Interval: time.Millisecond, Budget: time.Second, Batch: 7}
	runner, err := NewRetryDispatchRunner("qs.mysql", func(ctx context.Context, at time.Time, limit int) ([]app.Outcome, error) {
		if limit != 7 || at.IsZero() || at.Location() != time.UTC {
			t.Error("scan lost frozen time or batch bound")
		}
		deadline, bounded := ctx.Deadline()
		if !bounded || time.Until(deadline) > limits.Budget {
			t.Error("scan database work has no bounded context")
		}
		scans.Add(1)
		entered <- ctx
		<-release // A driver may still be running after cancellation.
		return nil, ctx.Err()
	}, func() bool {
		select {
		case checked <- struct{}{}:
		default:
		}
		return ready.Load()
	}, limits)
	if err != nil {
		t.Fatal(err)
	}
	if scans.Load() != 0 {
		t.Fatal("constructor started database work")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runner.Start(ctx)
	select {
	case <-checked:
	case <-time.After(time.Second):
		t.Fatal("admission was not checked")
	}
	if scans.Load() != 0 {
		t.Fatal("closed owner admission still scanned")
	}
	ready.Store(true)
	select {
	case scan := <-entered:
		cancel()
		select {
		case <-scan.Done():
		case <-time.After(time.Second):
			t.Fatal("cancellation did not reach actual scan")
		}
	case <-time.After(time.Second):
		t.Fatal("ready owner never scanned")
	}
	for i := 0; i < 5; i++ {
		runner.Start(ctx)
		if err := runner.Wait(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("claimed completion while driver is still running: %v", err)
		}
	}
	if scans.Load() != 1 {
		t.Fatal("cancellation/repeated start created additional work")
	}
	// Release before cleanup and join the actual loop; no extra waiter goroutine.
	release <- struct{}{}
	wait, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()
	if err := runner.Wait(wait); err != nil {
		t.Fatal(err)
	}
	if err := runner.Wait(ctx); err != nil {
		t.Fatal("completed loop reported pending", err)
	}
}

func TestRetryDispatchRunnerRejectsUnboundedOrForeignInputs(t *testing.T) {
	once := func(context.Context, time.Time, int) ([]app.Outcome, error) { return nil, nil }
	ready := func() bool { return true }
	limits := opts.RetryDispatchOptions{Interval: time.Second, Budget: time.Second, Batch: 50}
	for _, owner := range []string{"", "iam.mysql", "qs.mysql", "qs.mongo"} {
		r, err := NewRetryDispatchRunner(owner, once, ready, limits)
		if (owner == "qs.mysql" || owner == "qs.mongo") != (err == nil) {
			t.Fatal("wrong owner validation", owner, err)
		}
		if r != nil && r.Name() != "transaction_retry_dispatch_"+owner {
			t.Fatal("unstable scheduler identity")
		}
	}
	for _, invalid := range []opts.RetryDispatchOptions{{}, {Interval: time.Second, Budget: time.Second, Batch: 51}} {
		if _, err := NewRetryDispatchRunner("qs.mysql", once, ready, invalid); err == nil {
			t.Fatal("unbounded scan accepted")
		}
	}
	if _, err := NewRetryDispatchRunner("qs.mysql", nil, ready, limits); err == nil {
		t.Fatal("nil dispatcher accepted")
	}
	if _, err := NewRetryDispatchRunner("qs.mysql", once, nil, limits); err == nil {
		t.Fatal("missing owner admission accepted")
	}
}
