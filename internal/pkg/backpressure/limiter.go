package backpressure

import (
	"context"
	"time"

	basebackpressure "github.com/FangcunMount/component-base/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
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
	Observer   resilienceplane.Observer
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

func (l *Limiter) Snapshot(name string) resilienceplane.BackpressureSnapshot {
	if l == nil || l.limiter == nil {
		return resilienceplane.BackpressureSnapshot{
			Name:     name,
			Enabled:  false,
			Degraded: true,
			Reason:   "backpressure limiter disabled",
		}
	}
	stats := l.limiter.Stats(name)
	return resilienceplane.BackpressureSnapshot{
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
	observer resilienceplane.Observer
}

func (o resilienceObserver) OnBackpressure(ctx context.Context, event basebackpressure.Event) {
	subject := resilienceplane.Subject{
		Component: event.Component,
		Scope:     event.Dependency,
		Resource:  event.Resource,
		Strategy:  event.Strategy,
	}
	outcome := adaptOutcome(event.Outcome)
	if event.Outcome != basebackpressure.OutcomeReleased {
		resilienceplane.ObserveBackpressureWaitDuration(subject, outcome, event.Wait)
	}
	resilienceplane.Observe(ctx, o.observer, resilienceplane.ProtectionBackpressure, subject, outcome)
	if event.Outcome == basebackpressure.OutcomeAcquired || event.Outcome == basebackpressure.OutcomeReleased {
		resilienceplane.ObserveBackpressureInFlight(subject, event.InFlight)
	}
}

func adaptOutcome(outcome basebackpressure.Outcome) resilienceplane.Outcome {
	switch outcome {
	case basebackpressure.OutcomeAcquired:
		return resilienceplane.OutcomeBackpressureAcquired
	case basebackpressure.OutcomeReleased:
		return resilienceplane.OutcomeBackpressureReleased
	case basebackpressure.OutcomeTimeout:
		return resilienceplane.OutcomeBackpressureTimeout
	default:
		return resilienceplane.OutcomeBackpressureTimeout
	}
}

func defaultObserver(observer resilienceplane.Observer) resilienceplane.Observer {
	if observer != nil {
		return observer
	}
	return resilienceplane.DefaultObserver()
}
