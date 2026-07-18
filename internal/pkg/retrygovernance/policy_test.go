package retrygovernance

import (
	"testing"
	"time"
)

func TestPolicyDecideFailure(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	policy := Policy{
		Version:              "business-retry/v1",
		MaxAutomaticAttempts: 3,
		BaseDelay:            30 * time.Second,
		MaxDelay:             5 * time.Minute,
	}

	tests := []struct {
		name       string
		retryable  bool
		attempt    int
		want       Disposition
		wantDelay  time.Duration
		wantRemain int
	}{
		{name: "first retryable failure schedules second attempt", retryable: true, attempt: 1, want: DispositionAutomatic, wantDelay: 30 * time.Second, wantRemain: 2},
		{name: "second retryable failure schedules final automatic attempt", retryable: true, attempt: 2, want: DispositionAutomatic, wantDelay: time.Minute, wantRemain: 1},
		{name: "automatic budget exhaustion requires manual action", retryable: true, attempt: 3, want: DispositionManualRequired},
		{name: "manual attempt does not reset automatic budget", retryable: true, attempt: 4, want: DispositionManualRequired},
		{name: "non retryable failure is terminal", retryable: false, attempt: 1, want: DispositionTerminal, wantRemain: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := policy.DecideFailure(tt.retryable, tt.attempt, now)
			if decision.Disposition != tt.want {
				t.Fatalf("disposition = %q, want %q", decision.Disposition, tt.want)
			}
			if decision.RemainingAutomaticAttempts != tt.wantRemain {
				t.Fatalf("remaining attempts = %d, want %d", decision.RemainingAutomaticAttempts, tt.wantRemain)
			}
			if tt.wantDelay == 0 {
				if decision.NextAttemptAt != nil {
					t.Fatalf("next attempt = %s, want nil", decision.NextAttemptAt)
				}
				return
			}
			if decision.NextAttemptAt == nil || !decision.NextAttemptAt.Equal(now.Add(tt.wantDelay)) {
				t.Fatalf("next attempt = %v, want %s", decision.NextAttemptAt, now.Add(tt.wantDelay))
			}
		})
	}
}

func TestPolicyValidate(t *testing.T) {
	valid := Policy{Version: "v1", MaxAutomaticAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Minute}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid policy: %v", err)
	}

	invalid := []Policy{
		{},
		{Version: "v1", MaxAutomaticAttempts: 0, BaseDelay: time.Second, MaxDelay: time.Minute},
		{Version: "v1", MaxAutomaticAttempts: 3, BaseDelay: 0, MaxDelay: time.Minute},
		{Version: "v1", MaxAutomaticAttempts: 3, BaseDelay: time.Minute, MaxDelay: time.Second},
	}
	for _, policy := range invalid {
		if err := policy.Validate(); err == nil {
			t.Fatalf("policy %#v should be invalid", policy)
		}
	}
}

func TestAttemptOriginValidation(t *testing.T) {
	for _, origin := range []AttemptOrigin{AttemptOriginInitial, AttemptOriginAutomatic, AttemptOriginManual, AttemptOriginForce, AttemptOriginLeaseRecovery} {
		if !origin.IsValid() {
			t.Fatalf("origin %q should be valid", origin)
		}
	}
	if AttemptOrigin("unknown").IsValid() {
		t.Fatal("unknown origin should be invalid")
	}
}

func TestOutboxJitterIsDeterministicAndBounded(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	first := DefaultOutboxPolicy.DecideFailureForKey(true, 1, now, "evt-1")
	second := DefaultOutboxPolicy.DecideFailureForKey(true, 1, now, "evt-1")
	if first.NextAttemptAt == nil || second.NextAttemptAt == nil || !first.NextAttemptAt.Equal(*second.NextAttemptAt) {
		t.Fatalf("jitter is not deterministic: %v %v", first.NextAttemptAt, second.NextAttemptAt)
	}
	delay := first.NextAttemptAt.Sub(now)
	if delay < 8*time.Second || delay > 12*time.Second {
		t.Fatalf("delay = %s, want 10s +/-20%%", delay)
	}
}

func TestConfigurePoliciesPublishesAtomicSnapshots(t *testing.T) {
	originalBusiness, originalOutbox := BusinessPolicy(), OutboxPolicy()
	t.Cleanup(func() {
		if err := ConfigurePolicies(originalBusiness, originalOutbox); err != nil {
			t.Fatalf("restore policies: %v", err)
		}
	})
	business := Policy{Version: "business-test/v2", MaxAutomaticAttempts: 4, BaseDelay: time.Second, MaxDelay: time.Minute}
	outbox := Policy{Version: "outbox-test/v2", MaxAutomaticAttempts: 12, BaseDelay: 2 * time.Second, MaxDelay: time.Hour, JitterFraction: .1}
	if err := ConfigurePolicies(business, outbox); err != nil {
		t.Fatal(err)
	}
	if got := BusinessPolicy(); got.Version != business.Version || got.MaxAutomaticAttempts != 4 {
		t.Fatalf("business policy = %#v", got)
	}
	if got := OutboxPolicy(); got.Version != outbox.Version || got.MaxAutomaticAttempts != 12 {
		t.Fatalf("outbox policy = %#v", got)
	}
}
