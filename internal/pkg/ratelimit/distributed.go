package ratelimit

import (
	"context"

	baseratelimit "github.com/FangcunMount/component-base/pkg/ratelimit"
)

// DistributedLimiter adapts a shared token bucket backend into RateLimitDecision.
type DistributedLimiter struct {
	limiter baseratelimit.Limiter
}

func NewDistributedLimiter(backend Backend, policy RateLimitPolicy) *DistributedLimiter {
	return &DistributedLimiter{limiter: baseratelimit.NewDistributedLimiter(backend, policy.basePolicy())}
}

func (l *DistributedLimiter) Decide(ctx context.Context, key string) RateLimitDecision {
	if l == nil || l.limiter == nil {
		return allowedDecision(RateLimitPolicy{}, adaptOutcome(baseratelimit.OutcomeDegradedOpen))
	}
	return adaptDecision(l.limiter.Decide(ctx, key))
}
