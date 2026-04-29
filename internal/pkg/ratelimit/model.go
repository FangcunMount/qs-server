package ratelimit

import (
	"context"
	"time"

	baseratelimit "github.com/FangcunMount/component-base/pkg/ratelimit"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

// RateLimitPolicy describes one bounded rate limit control point.
type RateLimitPolicy struct {
	Component     string
	Scope         string
	Resource      string
	Strategy      string
	RatePerSecond float64
	Burst         int
}

// Subject returns the bounded resilience subject for this policy.
func (p RateLimitPolicy) Subject() resilienceplane.Subject {
	return resilienceplane.Subject{
		Component: p.Component,
		Scope:     p.Scope,
		Resource:  p.Resource,
		Strategy:  p.Strategy,
	}
}

func (p RateLimitPolicy) Valid() bool {
	return p.RatePerSecond > 0 && p.Burst > 0
}

func (p RateLimitPolicy) basePolicy() baseratelimit.Policy {
	return baseratelimit.Policy{
		Component:     p.Component,
		Scope:         p.Scope,
		Resource:      p.Resource,
		Strategy:      p.Strategy,
		RatePerSecond: p.RatePerSecond,
		Burst:         p.Burst,
	}
}

// RateLimitDecision is the transport-neutral outcome of one rate limit check.
type RateLimitDecision struct {
	Allowed           bool
	RetryAfter        time.Duration
	RetryAfterSeconds int
	Subject           resilienceplane.Subject
	Outcome           resilienceplane.Outcome
}

// RateLimiter decides whether one request key may pass.
type RateLimiter interface {
	Decide(ctx context.Context, key string) RateLimitDecision
}

type Backend = baseratelimit.Backend

func allowedDecision(policy RateLimitPolicy, outcome resilienceplane.Outcome) RateLimitDecision {
	return RateLimitDecision{
		Allowed: true,
		Subject: policy.Subject(),
		Outcome: outcome,
	}
}

func limitedDecision(policy RateLimitPolicy, retryAfter time.Duration, retryAfterSeconds int) RateLimitDecision {
	if retryAfterSeconds < 1 {
		retryAfterSeconds = 1
	}
	return RateLimitDecision{
		Allowed:           false,
		RetryAfter:        retryAfter,
		RetryAfterSeconds: retryAfterSeconds,
		Subject:           policy.Subject(),
		Outcome:           resilienceplane.OutcomeRateLimited,
	}
}

func adaptDecision(decision baseratelimit.Decision) RateLimitDecision {
	policy := RateLimitPolicy{
		Component:     decision.Policy.Component,
		Scope:         decision.Policy.Scope,
		Resource:      decision.Policy.Resource,
		Strategy:      decision.Policy.Strategy,
		RatePerSecond: decision.Policy.RatePerSecond,
		Burst:         decision.Policy.Burst,
	}
	if decision.Allowed {
		return allowedDecision(policy, adaptOutcome(decision.Outcome))
	}
	return RateLimitDecision{
		Allowed:           false,
		RetryAfter:        decision.RetryAfter,
		RetryAfterSeconds: decision.RetryAfterSeconds,
		Subject:           policy.Subject(),
		Outcome:           adaptOutcome(decision.Outcome),
	}
}

func adaptOutcome(outcome baseratelimit.Outcome) resilienceplane.Outcome {
	switch outcome {
	case baseratelimit.OutcomeAllowed:
		return resilienceplane.OutcomeAllowed
	case baseratelimit.OutcomeDegradedOpen:
		return resilienceplane.OutcomeDegradedOpen
	case baseratelimit.OutcomeRateLimited:
		return resilienceplane.OutcomeRateLimited
	default:
		return resilienceplane.OutcomeDegradedOpen
	}
}
