package run

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

func TestNextEvaluationRunIncrementsAttempt(t *testing.T) {
	t.Parallel()

	first := NewEvaluationRunWithAttempt(42, 1)
	if err := first.Start(time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := first.Fail(time.Now(), Failure{Kind: FailureKindTimeout, Message: "timed out", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	second := NextEvaluationRun(first)
	if second.Attempt().Number != 2 {
		t.Fatalf("attempt=%d, want 2", second.Attempt().Number)
	}
	if second.ID() != "42:2" {
		t.Fatalf("run id=%s", second.ID())
	}
}

func TestEvaluationRunLifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	run := NewEvaluationRun(42)
	if err := run.Start(now); err != nil {
		t.Fatal(err)
	}
	if run.Attempt().Status != StatusRunning {
		t.Fatalf("status=%s", run.Attempt().Status)
	}

	done := now.Add(time.Minute)
	if err := run.Succeed(done); err != nil {
		t.Fatal(err)
	}
	if run.Attempt().Status != StatusSucceeded {
		t.Fatalf("status=%s", run.Attempt().Status)
	}
	if run.FinishedAt() == nil || !run.FinishedAt().Equal(done) {
		t.Fatalf("finishedAt=%v", run.FinishedAt())
	}
}

func TestEvaluationRunFailureRetryable(t *testing.T) {
	t.Parallel()

	run := NewEvaluationRun(7)
	if err := run.Start(time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := run.Fail(time.Now(), Failure{Kind: FailureKindTimeout, Message: "timed out", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	if !run.Retryable() {
		t.Fatal("expected retryable failure")
	}
	decision := run.RetryDecision()
	if decision == nil || decision.Disposition != retrygovernance.DispositionAutomatic || decision.MaxAutomaticAttempts != 3 {
		t.Fatalf("retry decision = %#v", decision)
	}
}

func TestEvaluationRunRetryBudgetExhaustion(t *testing.T) {
	t.Parallel()

	now := time.Now()
	run := NewEvaluationRunWithAttempt(42, 3)
	if err := run.Start(now); err != nil {
		t.Fatal(err)
	}
	if err := run.Fail(now, Failure{Kind: FailureKindTimeout, Message: "timed out", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	if got := run.RetryDecision(); got == nil || got.Disposition != retrygovernance.DispositionManualRequired {
		t.Fatalf("retry decision = %#v, want manual_required", got)
	}
}

func TestEvaluationRunRejectsInvalidTransitionsAndSnapshotRewrites(t *testing.T) {
	t.Parallel()

	run := NewEvaluationRun(7)
	if err := run.Succeed(time.Now()); err == nil {
		t.Fatal("pending run must not succeed")
	}
	if err := run.Fail(time.Now(), Failure{Kind: FailureKindInternal, Message: "failed"}); err == nil {
		t.Fatal("pending run must not fail")
	}
	if err := run.Start(time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := run.Start(time.Now()); err == nil {
		t.Fatal("running run must not start again")
	}
	if err := run.AttachInputSnapshot("snapshot:v1"); err != nil {
		t.Fatal(err)
	}
	if err := run.AttachInputSnapshot("snapshot:v2"); err == nil {
		t.Fatal("run input snapshot must be immutable")
	}
	if err := run.Fail(time.Now(), Failure{Kind: FailureKindInternal, Message: "failed"}); err != nil {
		t.Fatal(err)
	}
	if err := run.Succeed(time.Now()); err == nil {
		t.Fatal("failed run must not succeed")
	}
}

func TestEvaluationRunClaimLeaseAndReclaim(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	run := NewEvaluationRun(42)
	if err := run.Claim(ClaimInput{Token: "worker-a", ClaimedAt: now, LeaseExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if run.Attempt().Status != StatusRunning || !run.HasActiveLease(now.Add(30*time.Second)) {
		t.Fatalf("claimed run = %#v, want active running lease", run)
	}
	if err := run.Claim(ClaimInput{Token: "worker-b", ClaimedAt: now.Add(2 * time.Minute), LeaseExpiresAt: now.Add(3 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if run.ClaimToken() != "worker-b" || !run.HasActiveLease(now.Add(2*time.Minute)) {
		t.Fatalf("reclaimed run = %#v, want worker-b ownership", run)
	}
	if err := run.Succeed(now.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if run.LeaseExpiresAt() != nil {
		t.Fatalf("terminal run retains lease: %v", run.LeaseExpiresAt())
	}
	if err := run.Claim(ClaimInput{Token: "worker-c", ClaimedAt: now.Add(4 * time.Minute), LeaseExpiresAt: now.Add(5 * time.Minute)}); err == nil {
		t.Fatal("terminal run must not be claimable")
	}
}

func TestEvaluationRunRejectsInvalidClaim(t *testing.T) {
	t.Parallel()

	now := time.Now()
	for _, tc := range []struct {
		name  string
		token string
		until time.Time
	}{
		{name: "empty token", until: now.Add(time.Minute)},
		{name: "expired lease", token: "worker", until: now},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := NewEvaluationRun(1)
			if err := run.Claim(ClaimInput{Token: tc.token, ClaimedAt: now, LeaseExpiresAt: tc.until}); err == nil {
				t.Fatal("expected invalid claim")
			}
		})
	}
}

func TestEvaluationRunManualAndForceAuthorizeExactlyOneAttempt(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		retryable bool
		origin    retrygovernance.AttemptOrigin
		want      retrygovernance.Disposition
	}{{"manual", true, retrygovernance.AttemptOriginManual, retrygovernance.DispositionManualRequired}, {"force", false, retrygovernance.AttemptOriginForce, retrygovernance.DispositionTerminal}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := NewEvaluationRunWithAttempt(9, 3)
			if err := run.Claim(ClaimInput{Token: "owner", ClaimedAt: now.Add(-time.Minute), LeaseExpiresAt: now.Add(time.Minute)}); err != nil {
				t.Fatal(err)
			}
			if err := run.Fail(now, Failure{Kind: FailureKindInternal, Message: "failed", Retryable: test.retryable}); err != nil {
				t.Fatal(err)
			}
			if got := run.RetryDecision().Disposition; got != test.want {
				t.Fatalf("initial disposition=%s want=%s", got, test.want)
			}
			if err := run.AuthorizeOneRetry(test.origin, "request-1", "event-1", now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			decision := run.RetryDecision()
			if decision.Disposition != retrygovernance.DispositionAutomatic || decision.ActionRequestID != "request-1" || decision.RetryEventID != "event-1" || decision.RemainingAutomaticAttempts != 0 {
				t.Fatalf("authorized decision=%#v", decision)
			}
			if err := run.AuthorizeOneRetry(test.origin, "request-2", "event-2", now.Add(2*time.Second)); err == nil {
				t.Fatal("second authorization unexpectedly succeeded")
			}
		})
	}
}
