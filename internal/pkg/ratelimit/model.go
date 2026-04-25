package ratelimit

import (
	"context"
	"time"

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
