package run

import (
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

func TestRunRequiresProviderResponseBeforeSuccess(t *testing.T) {
	startedAt := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	run := newStartedRun(t, startedAt)
	if err := run.Succeed(startedAt.Add(time.Second)); err == nil {
		t.Fatal("run succeeded before provider dispatch")
	}
	if err := run.BeginProviderDispatch(startedAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := run.RecordProviderResponse(validReceipt()); err != nil {
		t.Fatal(err)
	}
	if err := run.Succeed(startedAt.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if run.Status() != StatusSucceeded || run.InvocationPhase() != InvocationPhaseResponseReceived || run.LeaseExpiresAt() != nil {
		t.Fatalf("terminal run = status:%s phase:%s lease:%v", run.Status(), run.InvocationPhase(), run.LeaseExpiresAt())
	}
}

func TestRunRejectsUnsafePostDispatchLeaseReclaim(t *testing.T) {
	startedAt := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	run := newStartedRun(t, startedAt)
	if err := run.BeginProviderDispatch(startedAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	err := run.ReclaimExpiredLease(startedAt.Add(2*time.Minute), "recovery", startedAt.Add(3*time.Minute), false)
	if !errors.Is(err, ErrUnsafeLeaseReclaim) {
		t.Fatalf("reclaim error = %v, want ErrUnsafeLeaseReclaim", err)
	}
	if err := run.ReclaimExpiredLease(startedAt.Add(2*time.Minute), "recovery", startedAt.Add(3*time.Minute), true); err != nil {
		t.Fatal(err)
	}
	if run.InvocationID() != "generation-201/attempt-1" || run.RecoveryCount() != 1 {
		t.Fatal("idempotent recovery changed invocation identity")
	}
}

func TestRunSchedulesRecoveryWakeupOnlyForExactExpiredLease(t *testing.T) {
	startedAt := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	run := newStartedRun(t, startedAt)
	wakeup := RecoveryWakeup{
		EventID: "lease-recovery-1", ExpectedLeaseExpiresAt: startedAt.Add(time.Minute),
		InvocationPhase: InvocationPhasePrepared, RequestedAt: startedAt.Add(2 * time.Minute),
	}
	created, err := run.ScheduleRecoveryWakeup(wakeup)
	if err != nil || !created {
		t.Fatalf("schedule wake-up = created:%t error:%v", created, err)
	}
	created, err = run.ScheduleRecoveryWakeup(wakeup)
	if err != nil || created {
		t.Fatalf("idempotent wake-up = created:%t error:%v", created, err)
	}
	stale := wakeup
	stale.ExpectedLeaseExpiresAt = stale.ExpectedLeaseExpiresAt.Add(time.Second)
	if _, err := run.ScheduleRecoveryWakeup(stale); !errors.Is(err, ErrRecoveryNotAllowed) {
		t.Fatalf("stale wake-up error = %v", err)
	}
	if err := run.ReclaimExpiredLease(startedAt.Add(2*time.Minute), "recovery", startedAt.Add(3*time.Minute), false); err != nil {
		t.Fatal(err)
	}
	if run.RecoveryWakeup() != nil {
		t.Fatal("reclaimed lease retained stale recovery wake-up")
	}
}

func TestFailedRunCreatesNewAttempt(t *testing.T) {
	startedAt := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	run := newStartedRun(t, startedAt)
	if err := run.Fail(startedAt.Add(time.Second), Failure{
		Kind: FailureKindProviderTransport, Code: "provider_unavailable", SafeMessage: "AI 解读暂时不可用", Retryable: true,
	}); err != nil {
		t.Fatal(err)
	}
	next, err := Next(meta.FromUint64(302), run, retrygovernance.AttemptOriginAutomatic)
	if err != nil {
		t.Fatal(err)
	}
	if next.Attempt() != 2 || next.GenerationID() != run.GenerationID() || next.Status() != StatusPending {
		t.Fatalf("next run = attempt:%d generation:%s status:%s", next.Attempt(), next.GenerationID(), next.Status())
	}
}

func TestAuthorizeManualRetryEnforcesFailureRiskAndAttemptCeiling(t *testing.T) {
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		attempt    int
		failure    Failure
		acceptRisk bool
		wantErr    error
	}{
		{name: "retryable failure", attempt: 1, failure: Failure{Kind: FailureKindProviderTimeout, Code: "provider_timeout", SafeMessage: "暂时不可用", Retryable: true}},
		{name: "unknown result without accepted risk", attempt: 1, failure: Failure{Kind: FailureKindProviderTransport, Code: "provider_result_unknown", SafeMessage: "结果未知"}, wantErr: ErrRetryNotAllowed},
		{name: "unknown result with accepted risk", attempt: 1, failure: Failure{Kind: FailureKindProviderTransport, Code: "provider_result_unknown", SafeMessage: "结果未知"}, acceptRisk: true},
		{name: "non retryable validation failure", attempt: 1, failure: Failure{Kind: FailureKindOutputValidation, Code: "output_validation_failed", SafeMessage: "结果无效"}, wantErr: ErrRetryNotAllowed},
		{name: "hard maximum reached", attempt: retrygovernance.HardMaxBusinessAttempts, failure: Failure{Kind: FailureKindProviderTimeout, Code: "provider_timeout", SafeMessage: "暂时不可用", Retryable: true}, wantErr: ErrRetryNotAllowed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run, err := NewPending(meta.FromUint64(301), meta.FromUint64(201), test.attempt, retrygovernance.AttemptOriginInitial)
			if err != nil {
				t.Fatal(err)
			}
			if err := run.StartWithLease(now, "trace-1", now.Add(time.Minute), "invocation-1"); err != nil {
				t.Fatal(err)
			}
			if err := run.Fail(now.Add(time.Second), test.failure); err != nil {
				t.Fatal(err)
			}
			authorization := RetryAuthorization{
				ExpectedAttempt: test.attempt, NextAttempt: test.attempt + 1, Origin: retrygovernance.AttemptOriginManual,
				RequestID: "retry-request-1", EventID: "retry-event-1", Actor: "user:42", Reason: "manual recovery",
				AcceptedResultUnknownRisk: test.acceptRisk, AuthorizedAt: now.Add(2 * time.Second),
			}
			err = run.AuthorizeManualRetry(authorization)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("AuthorizeManualRetry error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil {
				stored := run.RetryAuthorization()
				if stored == nil || stored.RequestID != authorization.RequestID || stored.NextAttempt != test.attempt+1 {
					t.Fatalf("stored authorization = %#v", stored)
				}
			}
		})
	}
}

func TestAuthorizeManualRetryIsIdempotentOnlyForSameAuthorization(t *testing.T) {
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	run := newStartedRun(t, now)
	if err := run.Fail(now.Add(time.Second), Failure{Kind: FailureKindProviderTimeout, Code: "provider_timeout", SafeMessage: "暂时不可用", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	authorization := RetryAuthorization{ExpectedAttempt: 1, NextAttempt: 2, Origin: retrygovernance.AttemptOriginManual, RequestID: "retry-request-1", EventID: "retry-event-1", Actor: "user:42", Reason: "manual recovery", AuthorizedAt: now.Add(2 * time.Second)}
	if err := run.AuthorizeManualRetry(authorization); err != nil {
		t.Fatal(err)
	}
	if err := run.AuthorizeManualRetry(authorization); err != nil {
		t.Fatalf("same authorization should be idempotent: %v", err)
	}
	conflicting := authorization
	conflicting.Reason = "different decision under reused idempotency key"
	if err := run.AuthorizeManualRetry(conflicting); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting authorization error = %v", err)
	}
}

func newStartedRun(t *testing.T, startedAt time.Time) *AIExplanationRun {
	t.Helper()
	run, err := NewPending(meta.FromUint64(301), meta.FromUint64(201), 1, retrygovernance.AttemptOriginInitial)
	if err != nil {
		t.Fatal(err)
	}
	if err := run.StartWithLease(startedAt, "trace-1", startedAt.Add(time.Minute), "generation-201/attempt-1"); err != nil {
		t.Fatal(err)
	}
	return run
}

func validReceipt() aiexplanation.ProviderReceipt {
	return aiexplanation.ProviderReceipt{
		InvocationID: "generation-201/attempt-1", RequestID: "provider-request-1",
		Provider: "provider-a", Model: "model-a", InputTokens: 100, OutputTokens: 200, Latency: time.Second,
	}
}
