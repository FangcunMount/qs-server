package ratelimit

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
)

// DegradedFallbackLimiter consults fallback only when the primary limiter
// explicitly reports degraded-open. Healthy primary decisions remain
// authoritative and do not consume local fallback capacity.
type DegradedFallbackLimiter struct {
	primary  RateLimiter
	fallback RateLimiter
}

func NewDegradedFallbackLimiter(primary, fallback RateLimiter) *DegradedFallbackLimiter {
	return &DegradedFallbackLimiter{primary: primary, fallback: fallback}
}

func (l *DegradedFallbackLimiter) Decide(ctx context.Context, key string) RateLimitDecision {
	if l == nil || l.primary == nil {
		if l == nil || l.fallback == nil {
			return allowedDecision(RateLimitPolicy{}, resilience.OutcomeDegradedOpen)
		}
		return fallbackDecision(ctx, key, l.fallback)
	}

	primary := l.primary.Decide(ctx, key)
	if primary.Outcome != resilience.OutcomeDegradedOpen || l.fallback == nil {
		return primary
	}
	return fallbackDecision(ctx, key, l.fallback)
}

func fallbackDecision(ctx context.Context, key string, fallback RateLimiter) RateLimitDecision {
	decision := fallback.Decide(ctx, key)
	if decision.Allowed {
		decision.Outcome = resilience.OutcomeDegradedOpen
	}
	return decision
}
