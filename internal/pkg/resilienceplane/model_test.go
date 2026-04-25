package resilienceplane

import (
	"context"
	"testing"
	"time"
)

var _ Observer = NopObserver{}
var _ Observer = (*PrometheusObserver)(nil)

func TestNopObserverIsNilSafe(t *testing.T) {
	observer := NormalizeObserver(nil)
	observer.ObserveDecision(context.Background(), Decision{
		Kind:    ProtectionRateLimit,
		Subject: Subject{Component: "collection-server", Scope: "submit"},
		Outcome: OutcomeAllowed,
	})
}

func TestDefaultObserverIsPrometheusObserver(t *testing.T) {
	if _, ok := DefaultObserver().(PrometheusObserver); !ok {
		t.Fatalf("DefaultObserver() = %T, want PrometheusObserver", DefaultObserver())
	}
}

func TestOutcomeStringValuesAreStable(t *testing.T) {
	cases := map[Outcome]string{
		OutcomeAllowed:              "allowed",
		OutcomeRateLimited:          "rate_limited",
		OutcomeDegradedOpen:         "degraded_open",
		OutcomeQueueAccepted:        "queue_accepted",
		OutcomeQueueFull:            "queue_full",
		OutcomeQueueDuplicate:       "queue_duplicate",
		OutcomeQueueProcessing:      "queue_processing",
		OutcomeQueueDone:            "queue_done",
		OutcomeQueueFailed:          "queue_failed",
		OutcomeQueueStatusCleaned:   "queue_status_cleaned",
		OutcomeBackpressureAcquired: "backpressure_acquired",
		OutcomeBackpressureTimeout:  "backpressure_timeout",
		OutcomeBackpressureReleased: "backpressure_released",
		OutcomeLockAcquired:         "lock_acquired",
		OutcomeLockContention:       "lock_contention",
		OutcomeLockReleased:         "lock_released",
		OutcomeLockError:            "lock_error",
		OutcomeLockDegraded:         "lock_degraded",
		OutcomeIdempotencyHit:       "idempotency_hit",
		OutcomeDuplicateSkipped:     "duplicate_skipped",
	}
	for outcome, want := range cases {
		if got := outcome.String(); got != want {
			t.Fatalf("outcome %v string = %q, want %q", outcome, got, want)
		}
	}
}

func TestMetricHelpersNormalizeBoundedLabels(t *testing.T) {
	subject := Subject{
		Component: "collection-server",
		Scope:     "answersheet_submit",
		Resource:  "submit_queue",
		Strategy:  "memory_channel",
	}

	ObserveQueueDepth(subject, 3)
	ObserveQueueStatus(subject, "queued", 2)
	ObserveBackpressureInFlight(Subject{
		Component: "apiserver",
		Scope:     "mysql",
		Resource:  "downstream",
		Strategy:  "semaphore",
	}, 1)
	ObserveBackpressureWaitDuration(Subject{
		Component: "apiserver",
		Scope:     "mysql",
		Resource:  "downstream",
		Strategy:  "semaphore",
	}, OutcomeBackpressureAcquired, 0)
}

func TestFinalizeRuntimeSnapshotSummarizesCapabilities(t *testing.T) {
	snapshot := NewRuntimeSnapshot("apiserver", time.Time{})
	snapshot.RateLimits = []CapabilitySnapshot{{Name: "rest", Configured: true}}
	snapshot.Backpressure = []BackpressureSnapshot{{Name: "mysql", Enabled: true}, {Name: "mongo", Degraded: true}}

	got := FinalizeRuntimeSnapshot(snapshot)
	if got.Summary.CapabilityCount != 3 {
		t.Fatalf("capability count = %d, want 3", got.Summary.CapabilityCount)
	}
	if got.Summary.DegradedCount != 1 || got.Summary.Ready {
		t.Fatalf("summary = %+v, want one degraded and not ready", got.Summary)
	}
}

func TestProtectionKindStringValuesAreStable(t *testing.T) {
	cases := map[ProtectionKind]string{
		ProtectionRateLimit:            "rate_limit",
		ProtectionQueue:                "queue",
		ProtectionBackpressure:         "backpressure",
		ProtectionLock:                 "lock",
		ProtectionIdempotency:          "idempotency",
		ProtectionDuplicateSuppression: "duplicate_suppression",
	}
	for kind, want := range cases {
		if got := kind.String(); got != want {
			t.Fatalf("kind %v string = %q, want %q", kind, got, want)
		}
	}
}

func TestObserveNormalizesEmptySubject(t *testing.T) {
	observer := &recordingObserver{}
	Observe(context.Background(), observer, ProtectionQueue, Subject{}, OutcomeQueueAccepted)
	if len(observer.decisions) != 1 {
		t.Fatalf("got %d decisions, want 1", len(observer.decisions))
	}
	got := observer.decisions[0].Subject
	if got.Component != "unknown" || got.Scope != "default" || got.Resource != "default" || got.Strategy != "default" {
		t.Fatalf("normalized subject = %+v", got)
	}
}

type recordingObserver struct {
	decisions []Decision
}

func (r *recordingObserver) ObserveDecision(_ context.Context, decision Decision) {
	r.decisions = append(r.decisions, decision)
}
