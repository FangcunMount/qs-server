package resilienceplane

import "context"

// ProtectionKind classifies one resilience control point.
type ProtectionKind string

const (
	ProtectionRateLimit            ProtectionKind = "rate_limit"
	ProtectionQueue                ProtectionKind = "queue"
	ProtectionBackpressure         ProtectionKind = "backpressure"
	ProtectionLock                 ProtectionKind = "lock"
	ProtectionIdempotency          ProtectionKind = "idempotency"
	ProtectionDuplicateSuppression ProtectionKind = "duplicate_suppression"
)

func (k ProtectionKind) String() string { return string(k) }

// Outcome is a bounded result label for one resilience decision.
type Outcome string

const (
	OutcomeAllowed              Outcome = "allowed"
	OutcomeRateLimited          Outcome = "rate_limited"
	OutcomeDegradedOpen         Outcome = "degraded_open"
	OutcomeQueueAccepted        Outcome = "queue_accepted"
	OutcomeQueueFull            Outcome = "queue_full"
	OutcomeQueueDuplicate       Outcome = "queue_duplicate"
	OutcomeQueueProcessing      Outcome = "queue_processing"
	OutcomeQueueDone            Outcome = "queue_done"
	OutcomeQueueFailed          Outcome = "queue_failed"
	OutcomeQueueStatusCleaned   Outcome = "queue_status_cleaned"
	OutcomeBackpressureAcquired Outcome = "backpressure_acquired"
	OutcomeBackpressureTimeout  Outcome = "backpressure_timeout"
	OutcomeBackpressureReleased Outcome = "backpressure_released"
	OutcomeLockAcquired         Outcome = "lock_acquired"
	OutcomeLockContention       Outcome = "lock_contention"
	OutcomeLockReleased         Outcome = "lock_released"
	OutcomeLockError            Outcome = "lock_error"
	OutcomeLockDegraded         Outcome = "lock_degraded"
	OutcomeIdempotencyHit       Outcome = "idempotency_hit"
	OutcomeDuplicateSkipped     Outcome = "duplicate_skipped"
)

func (o Outcome) String() string { return string(o) }

// Subject identifies a bounded resilience control point. Do not put user IDs,
// request IDs, lock keys, or other high-cardinality values in these fields.
type Subject struct {
	Component string
	Scope     string
	Resource  string
	Strategy  string
}

// Decision describes one resilience outcome.
type Decision struct {
	Kind    ProtectionKind
	Subject Subject
	Outcome Outcome
}

// Observer receives resilience decisions. Implementations must keep labels
// bounded and must not affect business behavior.
type Observer interface {
	ObserveDecision(context.Context, Decision)
}

type NopObserver struct{}

func (NopObserver) ObserveDecision(context.Context, Decision) {}

func NormalizeObserver(observer Observer) Observer {
	if observer == nil {
		return NopObserver{}
	}
	return observer
}

func DefaultObserver() Observer {
	return PrometheusObserver{}
}

func Observe(ctx context.Context, observer Observer, kind ProtectionKind, subject Subject, outcome Outcome) {
	NormalizeObserver(observer).ObserveDecision(ctx, Decision{
		Kind:    kind,
		Subject: normalizeSubject(subject),
		Outcome: outcome,
	})
}

func normalizeSubject(subject Subject) Subject {
	if subject.Component == "" {
		subject.Component = "unknown"
	}
	if subject.Scope == "" {
		subject.Scope = "default"
	}
	if subject.Resource == "" {
		subject.Resource = "default"
	}
	if subject.Strategy == "" {
		subject.Strategy = "default"
	}
	return subject
}
