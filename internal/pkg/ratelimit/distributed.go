package ratelimit

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

type Backend interface {
	Allow(ctx context.Context, key string, ratePerSecond float64, burst int) (bool, time.Duration, error)
}

// DistributedLimiter adapts a shared token bucket backend into RateLimitDecision.
type DistributedLimiter struct {
	backend Backend
	policy  RateLimitPolicy
}

func NewDistributedLimiter(backend Backend, policy RateLimitPolicy) *DistributedLimiter {
	return &DistributedLimiter{backend: backend, policy: policy}
}

func (l *DistributedLimiter) Decide(ctx context.Context, key string) RateLimitDecision {
	if l == nil || l.backend == nil || key == "" || !l.policy.Valid() {
		return allowedDecision(l.policy, resilienceplane.OutcomeDegradedOpen)
	}
	allowed, retryAfter, err := l.backend.Allow(ctx, key, l.policy.RatePerSecond, l.policy.Burst)
	if err != nil {
		return allowedDecision(l.policy, resilienceplane.OutcomeDegradedOpen)
	}
	if allowed {
		return allowedDecision(l.policy, resilienceplane.OutcomeAllowed)
	}
	seconds := int(retryAfter.Seconds()) + 1
	if seconds < 1 {
		seconds = 1
	}
	return limitedDecision(l.policy, retryAfter, seconds)
}
