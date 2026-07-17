package backpressure

import (
	"context"
	"time"

	basebackpressure "github.com/FangcunMount/component-base/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
)

// Limiter provides a simple in-flight limiter with optional timeout.
type Limiter struct {
	limiter *basebackpressure.Limiter
}

type Acquirer interface {
	Acquire(context.Context) (context.Context, func(), error)
}

type Options struct {
	Component  string
	Dependency string
	Observer   resilience.Observer
}

// NewLimiter creates a limiter with maxInflight and optional timeout.
func NewLimiter(maxInflight int, timeout time.Duration) *Limiter {
	return NewLimiterWithOptions(maxInflight, timeout, Options{})
}

func NewLimiterWithOptions(maxInflight int, timeout time.Duration, opts Options) *Limiter {
	limiter := basebackpressure.NewLimiterWithOptions(maxInflight, timeout, basebackpressure.Options{
		Component:  opts.Component,
		Dependency: opts.Dependency,
		Observer:   resilienceObserver{observer: defaultObserver(opts.Observer)},
	})
	if limiter == nil {
		return nil
	}
	return &Limiter{limiter: limiter}
}

// Acquire waits for a slot or until context timeout/cancel.
// It returns a possibly wrapped context and a release function.
func (l *Limiter) Acquire(ctx context.Context) (context.Context, func(), error) {
	if l == nil || l.limiter == nil {
		return ctx, func() {}, nil
	}
	return l.limiter.Acquire(ctx)
}

func (l *Limiter) Snapshot(name string) resilience.BackpressureSnapshot {
	if l == nil || l.limiter == nil {
		return resilience.BackpressureSnapshot{
			Name:     name,
			Enabled:  false,
			Degraded: true,
			Reason:   "backpressure limiter disabled",
		}
	}
	stats := l.limiter.Stats(name)
	return resilience.BackpressureSnapshot{
		Component:     stats.Component,
		Name:          stats.Name,
		Dependency:    stats.Dependency,
		Strategy:      stats.Strategy,
		Enabled:       stats.Enabled,
		MaxInflight:   stats.MaxInflight,
		InFlight:      stats.InFlight,
		TimeoutMillis: stats.TimeoutMillis,
		Degraded:      stats.Degraded,
		Reason:        stats.Reason,
	}
}

type resilienceObserver struct {
	observer resilience.Observer
}

func (o resilienceObserver) OnBackpressure(ctx context.Context, event basebackpressure.Event) {
	subject := resilience.Subject{
		Component: event.Component,
		Scope:     event.Dependency,
		Resource:  event.Resource,
		Strategy:  event.Strategy,
	}
	outcome := adaptOutcome(event.Outcome)
	if event.Outcome != basebackpressure.OutcomeReleased {
		resilience.ObserveBackpressureWaitDuration(subject, outcome, event.Wait)
	}
	resilience.Observe(ctx, o.observer, resilience.ProtectionBackpressure, subject, outcome)
	if event.Outcome == basebackpressure.OutcomeAcquired || event.Outcome == basebackpressure.OutcomeReleased {
		resilience.ObserveBackpressureInFlight(subject, event.InFlight)
	}
}

func adaptOutcome(outcome basebackpressure.Outcome) resilience.Outcome {
	switch outcome {
	case basebackpressure.OutcomeAcquired:
		return resilience.OutcomeBackpressureAcquired
	case basebackpressure.OutcomeReleased:
		return resilience.OutcomeBackpressureReleased
	case basebackpressure.OutcomeTimeout:
		return resilience.OutcomeBackpressureTimeout
	default:
		return resilience.OutcomeBackpressureTimeout
	}
}

func defaultObserver(observer resilience.Observer) resilience.Observer {
	if observer != nil {
		return observer
	}
	return resilience.DefaultObserver()
}
