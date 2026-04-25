package ratelimit

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

type RedisBackend interface {
	Allow(ctx context.Context, key string, ratePerSecond float64, burst int) (bool, time.Duration, error)
}

// RedisLimiter adapts a Redis token bucket primitive into RateLimitDecision.
type RedisLimiter struct {
	backend RedisBackend
	policy  RateLimitPolicy
}

func NewRedisLimiter(backend RedisBackend, policy RateLimitPolicy) *RedisLimiter {
	return &RedisLimiter{backend: backend, policy: policy}
}

func (l *RedisLimiter) Decide(ctx context.Context, key string) RateLimitDecision {
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
