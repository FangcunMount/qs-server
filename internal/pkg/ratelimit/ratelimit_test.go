package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

func TestLocalLimiterAllowsThenLimitsWithRetryAfter(t *testing.T) {
	limiter := NewLocalLimiter(testPolicy("local"))

	first := limiter.Decide(context.Background(), "")
	if !first.Allowed || first.Outcome != resilienceplane.OutcomeAllowed {
		t.Fatalf("first decision = %#v, want allowed", first)
	}

	second := limiter.Decide(context.Background(), "")
	if second.Allowed {
		t.Fatalf("second decision = %#v, want limited", second)
	}
	if second.Outcome != resilienceplane.OutcomeRateLimited {
		t.Fatalf("outcome = %s, want %s", second.Outcome, resilienceplane.OutcomeRateLimited)
	}
	if second.RetryAfterSeconds < 1 {
		t.Fatalf("retryAfterSeconds = %d, want positive", second.RetryAfterSeconds)
	}
}

func TestKeyedLocalLimiterIsIndependentPerKeyAndUsesAnonymousFallback(t *testing.T) {
	limiter := NewKeyedLocalLimiter(testPolicy("local_key"))

	if !limiter.Decide(context.Background(), "a").Allowed {
		t.Fatal("first key a decision should be allowed")
	}
	if limiter.Decide(context.Background(), "a").Allowed {
		t.Fatal("second key a decision should be limited")
	}
	if !limiter.Decide(context.Background(), "b").Allowed {
		t.Fatal("first key b decision should be allowed")
	}
	if !limiter.Decide(context.Background(), "").Allowed {
		t.Fatal("first anonymous decision should be allowed")
	}
	if limiter.Decide(context.Background(), "").Allowed {
		t.Fatal("second anonymous decision should be limited")
	}
}

func TestLocalLimiterInvalidPolicyRejectsWithRetryAfter(t *testing.T) {
	limiter := NewLocalLimiter(RateLimitPolicy{Strategy: "local"})
	decision := limiter.Decide(context.Background(), "")
	if decision.Allowed {
		t.Fatalf("decision = %#v, want rejected", decision)
	}
	if decision.RetryAfterSeconds != 1 {
		t.Fatalf("retryAfterSeconds = %d, want 1", decision.RetryAfterSeconds)
	}
}

func TestRedisLimiterAllowsLimitsAndDegradesOpen(t *testing.T) {
	backend := &fakeRedisBackend{
		allowed:    false,
		retryAfter: 1500 * time.Millisecond,
	}
	limiter := NewRedisLimiter(backend, testPolicy("redis"))

	decision := limiter.Decide(context.Background(), "limit:submit:global")
	if decision.Allowed {
		t.Fatalf("decision = %#v, want limited", decision)
	}
	if decision.RetryAfterSeconds != 2 {
		t.Fatalf("retryAfterSeconds = %d, want 2", decision.RetryAfterSeconds)
	}

	backend.allowed = true
	decision = limiter.Decide(context.Background(), "limit:submit:global")
	if !decision.Allowed || decision.Outcome != resilienceplane.OutcomeAllowed {
		t.Fatalf("decision = %#v, want allowed", decision)
	}

	backend.err = errors.New("redis down")
	decision = limiter.Decide(context.Background(), "limit:submit:global")
	if !decision.Allowed || decision.Outcome != resilienceplane.OutcomeDegradedOpen {
		t.Fatalf("decision = %#v, want degraded open", decision)
	}

	decision = NewRedisLimiter(nil, testPolicy("redis")).Decide(context.Background(), "limit:submit:global")
	if !decision.Allowed || decision.Outcome != resilienceplane.OutcomeDegradedOpen {
		t.Fatalf("nil backend decision = %#v, want degraded open", decision)
	}
}

func testPolicy(strategy string) RateLimitPolicy {
	return RateLimitPolicy{
		Component:     "test",
		Scope:         "submit",
		Resource:      "global",
		Strategy:      strategy,
		RatePerSecond: 1,
		Burst:         1,
	}
}

type fakeRedisBackend struct {
	allowed    bool
	retryAfter time.Duration
	err        error
}

func (f *fakeRedisBackend) Allow(context.Context, string, float64, int) (bool, time.Duration, error) {
	return f.allowed, f.retryAfter, f.err
}
