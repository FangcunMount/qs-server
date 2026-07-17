package ratelimit

import (
	"context"
	"time"

	baseratelimit "github.com/FangcunMount/component-base/pkg/ratelimit"
)

// LocalLimiter adapts component-base's in-process limiter to qs resilience decisions.
type LocalLimiter struct {
	limiter baseratelimit.Limiter
}

func NewLocalLimiter(policy RateLimitPolicy) *LocalLimiter {
	return &LocalLimiter{limiter: baseratelimit.NewLocalLimiter(policy.basePolicy())}
}

func NewKeyedLocalLimiter(policy RateLimitPolicy) *LocalLimiter {
	return &LocalLimiter{limiter: baseratelimit.NewKeyedLocalLimiter(policy.basePolicy())}
}

func (l *LocalLimiter) Decide(ctx context.Context, key string) RateLimitDecision {
	if l == nil || l.limiter == nil {
		return limitedDecision(RateLimitPolicy{}, time.Second, 1)
	}
	return adaptDecision(l.limiter.Decide(ctx, key))
}
