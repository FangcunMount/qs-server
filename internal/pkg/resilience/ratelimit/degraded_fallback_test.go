package ratelimit

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
)

func TestDegradedFallbackLimiterUsesFallbackOnlyForDegradedOpen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		primary       RateLimitDecision
		fallback      RateLimitDecision
		wantAllowed   bool
		wantOutcome   resilience.Outcome
		wantFallbacks int
	}{
		{
			name: "healthy primary allowed",
			primary: RateLimitDecision{
				Allowed: true,
				Outcome: resilience.OutcomeAllowed,
			},
			fallback:      RateLimitDecision{Allowed: false, Outcome: resilience.OutcomeRateLimited},
			wantAllowed:   true,
			wantOutcome:   resilience.OutcomeAllowed,
			wantFallbacks: 0,
		},
		{
			name: "healthy primary limited",
			primary: RateLimitDecision{
				Allowed: false,
				Outcome: resilience.OutcomeRateLimited,
			},
			fallback:      RateLimitDecision{Allowed: true, Outcome: resilience.OutcomeAllowed},
			wantAllowed:   false,
			wantOutcome:   resilience.OutcomeRateLimited,
			wantFallbacks: 0,
		},
		{
			name: "degraded primary fallback allowed",
			primary: RateLimitDecision{
				Allowed: true,
				Outcome: resilience.OutcomeDegradedOpen,
			},
			fallback:      RateLimitDecision{Allowed: true, Outcome: resilience.OutcomeAllowed},
			wantAllowed:   true,
			wantOutcome:   resilience.OutcomeDegradedOpen,
			wantFallbacks: 1,
		},
		{
			name: "degraded primary fallback limited",
			primary: RateLimitDecision{
				Allowed: true,
				Outcome: resilience.OutcomeDegradedOpen,
			},
			fallback:      RateLimitDecision{Allowed: false, Outcome: resilience.OutcomeRateLimited},
			wantAllowed:   false,
			wantOutcome:   resilience.OutcomeRateLimited,
			wantFallbacks: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := &staticLimiter{decision: tt.primary}
			fallback := &staticLimiter{decision: tt.fallback}
			limiter := NewDegradedFallbackLimiter(primary, fallback)

			got := limiter.Decide(t.Context(), "submit")
			if got.Allowed != tt.wantAllowed || got.Outcome != tt.wantOutcome {
				t.Fatalf("Decide() = %#v, want allowed=%v outcome=%s", got, tt.wantAllowed, tt.wantOutcome)
			}
			if fallback.calls != tt.wantFallbacks {
				t.Fatalf("fallback calls = %d, want %d", fallback.calls, tt.wantFallbacks)
			}
		})
	}
}

type staticLimiter struct {
	decision RateLimitDecision
	calls    int
}

func (l *staticLimiter) Decide(context.Context, string) RateLimitDecision {
	l.calls++
	return l.decision
}
