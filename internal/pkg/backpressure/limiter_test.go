package backpressure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

func TestAcquireDoesNotWrapOperationContextWithLimiterTimeout(t *testing.T) {
	limiter := NewLimiter(1, 50*time.Millisecond)

	ctx := context.Background()
	gotCtx, release, err := limiter.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer release()

	if gotCtx != ctx {
		t.Fatalf("Acquire() should preserve original context")
	}
	if _, ok := gotCtx.Deadline(); ok {
		t.Fatalf("Acquire() should not add a deadline to the downstream operation context")
	}
}

func TestAcquireTimeoutOnlyAppliesWhileWaitingForSlot(t *testing.T) {
	limiter := NewLimiter(1, 50*time.Millisecond)

	_, release, err := limiter.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer release()

	_, _, err = limiter.Acquire(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire() error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestAcquireReportsOutcomes(t *testing.T) {
	observer := &backpressureRecordingObserver{}
	limiter := NewLimiterWithOptions(1, 10*time.Millisecond, Options{
		Component:  "apiserver",
		Dependency: "mysql",
		Observer:   observer,
	})

	_, release, err := limiter.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	release()

	if !observer.has(resilienceplane.OutcomeBackpressureAcquired) {
		t.Fatal("expected acquired outcome")
	}
	if !observer.has(resilienceplane.OutcomeBackpressureReleased) {
		t.Fatal("expected released outcome")
	}
}

func TestAcquireTimeoutReportsOutcome(t *testing.T) {
	observer := &backpressureRecordingObserver{}
	limiter := NewLimiterWithOptions(1, 10*time.Millisecond, Options{Observer: observer})

	_, release, err := limiter.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer release()

	if _, _, err := limiter.Acquire(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Acquire() error = %v, want deadline exceeded", err)
	}
	if !observer.has(resilienceplane.OutcomeBackpressureTimeout) {
		t.Fatal("expected timeout outcome")
	}
}

func TestSnapshotReportsInFlightAndConfig(t *testing.T) {
	limiter := NewLimiterWithOptions(2, 150*time.Millisecond, Options{
		Component:  "apiserver",
		Dependency: "mysql",
	})

	snapshot := limiter.Snapshot("mysql")
	if !snapshot.Enabled || snapshot.MaxInflight != 2 || snapshot.TimeoutMillis != 150 {
		t.Fatalf("initial snapshot = %+v", snapshot)
	}
	if snapshot.InFlight != 0 {
		t.Fatalf("initial in-flight = %d, want 0", snapshot.InFlight)
	}

	_, release, err := limiter.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer release()

	snapshot = limiter.Snapshot("mysql")
	if snapshot.InFlight != 1 {
		t.Fatalf("in-flight = %d, want 1", snapshot.InFlight)
	}
}

func TestNilLimiterSnapshotIsDegraded(t *testing.T) {
	var limiter *Limiter
	snapshot := limiter.Snapshot("mysql")
	if snapshot.Enabled || !snapshot.Degraded {
		t.Fatalf("nil snapshot = %+v, want disabled degraded", snapshot)
	}
}

type backpressureRecordingObserver struct {
	decisions []resilienceplane.Decision
}

func (r *backpressureRecordingObserver) ObserveDecision(_ context.Context, decision resilienceplane.Decision) {
	r.decisions = append(r.decisions, decision)
}

func (r *backpressureRecordingObserver) has(outcome resilienceplane.Outcome) bool {
	for _, decision := range r.decisions {
		if decision.Outcome == outcome {
			return true
		}
	}
	return false
}
