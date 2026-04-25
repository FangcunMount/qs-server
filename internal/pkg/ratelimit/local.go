package ratelimit

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"golang.org/x/time/rate"
)

const (
	keyedLimiterTTL      = 10 * time.Minute
	keyedCleanupInterval = 1 * time.Minute
	keyedMaxEntries      = 10000
)

type keyedEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// LocalLimiter is an in-process token bucket limiter.
type LocalLimiter struct {
	policy RateLimitPolicy
	keyed  bool

	global *rate.Limiter

	mu          sync.Mutex
	entries     map[string]*keyedEntry
	lastCleanup time.Time
}

func NewLocalLimiter(policy RateLimitPolicy) *LocalLimiter {
	return &LocalLimiter{
		policy: policy,
		global: newRateLimiter(policy),
	}
}

func NewKeyedLocalLimiter(policy RateLimitPolicy) *LocalLimiter {
	return &LocalLimiter{
		policy:  policy,
		keyed:   true,
		entries: make(map[string]*keyedEntry),
	}
}

func (l *LocalLimiter) Decide(_ context.Context, key string) RateLimitDecision {
	if l == nil {
		return limitedDecision(RateLimitPolicy{}, time.Second, 1)
	}
	if !l.policy.Valid() {
		return limitedDecision(l.policy, time.Second, 1)
	}

	limiter := l.global
	if l.keyed {
		limiter = l.limiterForKey(key)
	}
	if limiter.Allow() {
		return allowedDecision(l.policy, resilienceplane.OutcomeAllowed)
	}
	retryAfter, seconds := retryAfterForLimiter(limiter)
	return limitedDecision(l.policy, retryAfter, seconds)
}

func (l *LocalLimiter) limiterForKey(key string) *rate.Limiter {
	if key == "" {
		key = "anonymous"
	}

	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastCleanup) >= keyedCleanupInterval {
		for k, entry := range l.entries {
			if now.Sub(entry.lastSeen) > keyedLimiterTTL {
				delete(l.entries, k)
			}
		}
		if len(l.entries) > keyedMaxEntries {
			for k := range l.entries {
				delete(l.entries, k)
				if len(l.entries) <= keyedMaxEntries {
					break
				}
			}
		}
		l.lastCleanup = now
	}

	entry := l.entries[key]
	if entry == nil {
		entry = &keyedEntry{limiter: newRateLimiter(l.policy)}
		l.entries[key] = entry
	}
	entry.lastSeen = now
	return entry.limiter
}

func newRateLimiter(policy RateLimitPolicy) *rate.Limiter {
	if !policy.Valid() {
		return rate.NewLimiter(0, 0)
	}
	return rate.NewLimiter(rate.Limit(policy.RatePerSecond), policy.Burst)
}

func retryAfterForLimiter(limiter *rate.Limiter) (time.Duration, int) {
	if limiter == nil {
		return time.Second, 1
	}
	reservation := limiter.Reserve()
	if !reservation.OK() {
		return time.Second, 1
	}
	delay := reservation.Delay()
	reservation.CancelAt(time.Now())
	seconds := int(math.Ceil(delay.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return delay, seconds
}
