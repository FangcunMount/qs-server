package run

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

func TestInterpretationRunRecordsOneAttemptAndCreatesNextOnlyAfterFailure(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	first, err := NewPending(meta.FromUint64(11), meta.FromUint64(7), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Start(now, "trace-1"); err != nil {
		t.Fatal(err)
	}
	failure := Failure{Kind: FailureKindTemplate, Code: "template_unavailable", SafeMessage: "报告模板暂不可用", Retryable: true}
	if err := first.Fail(now.Add(time.Second), failure); err != nil {
		t.Fatal(err)
	}
	if got := first.RetryDecision(); got == nil || got.Disposition != retrygovernance.DispositionAutomatic {
		t.Fatalf("retry decision = %#v", got)
	}
	second, err := Next(meta.FromUint64(12), first)
	if err != nil {
		t.Fatal(err)
	}
	if second.GenerationID() != first.GenerationID() || second.Attempt() != 2 || second.Status() != StatusPending {
		t.Fatalf("next run = generation:%s attempt:%d status:%s", second.GenerationID(), second.Attempt(), second.Status())
	}
	if err := second.Start(now.Add(2*time.Second), "trace-2"); err != nil {
		t.Fatal(err)
	}
	if err := second.Succeed(now.Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := Next(meta.FromUint64(13), second); err == nil {
		t.Fatal("succeeded run produced another attempt")
	}
}

func TestInterpretationRunRejectsInvalidTransitionsAndFailure(t *testing.T) {
	now := time.Now()
	r, err := NewPending(meta.FromUint64(1), meta.FromUint64(2), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Succeed(now); err == nil {
		t.Fatal("pending run succeeded")
	}
	if err := r.Start(now, ""); err != nil {
		t.Fatal(err)
	}
	if err := r.Fail(now, Failure{}); err == nil {
		t.Fatal("run accepted an unclassified failure")
	}
}

func TestRestoreRunRejectsInconsistentTerminalFacts(t *testing.T) {
	now := time.Now()
	if _, err := Restore(RestoreInput{ID: meta.FromUint64(1), GenerationID: meta.FromUint64(2), Attempt: 1, Status: StatusSucceeded, StartedAt: &now}); err == nil {
		t.Fatal("succeeded run restored without finished at")
	}
	finished := now.Add(time.Second)
	failure := Failure{Kind: FailureKindBuild, Code: "builder_error", SafeMessage: "报告生成失败"}
	restored, err := Restore(RestoreInput{ID: meta.FromUint64(1), GenerationID: meta.FromUint64(2), Attempt: 1, Status: StatusFailed, Failure: &failure, StartedAt: &now, FinishedAt: &finished})
	if err != nil || restored.Failure() == nil || restored.Failure().Code != "builder_error" {
		t.Fatalf("restore = run:%#v err:%v", restored, err)
	}
}

func TestInterpretationRunLeaseExpiresAndIsClearedAtTerminalState(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	r, err := NewPending(meta.FromUint64(1), meta.FromUint64(2), 1)
	if err != nil {
		t.Fatal(err)
	}
	leaseExpiry := now.Add(time.Minute)
	if err := r.StartWithLease(now, "trace", leaseExpiry); err != nil {
		t.Fatal(err)
	}
	if !r.HasActiveLease(now.Add(30*time.Second)) || r.HasActiveLease(leaseExpiry) {
		t.Fatalf("lease state = expires:%v", r.LeaseExpiresAt())
	}
	if err := r.Succeed(now.Add(40 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if r.LeaseExpiresAt() != nil || r.HasActiveLease(now.Add(45*time.Second)) {
		t.Fatal("terminal run retained active lease")
	}
}

func TestInterpretationRunManualAuthorizationDoesNotResetBudget(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	run, err := NewPending(meta.FromUint64(13), meta.FromUint64(9), 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := run.StartWithLease(now.Add(-time.Minute), "trace", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := run.Fail(now, Failure{Kind: FailureKindBuild, Code: "failed", SafeMessage: "failed", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	if err := run.AuthorizeOneRetry(retrygovernance.AttemptOriginManual, "request-1", "event-1", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	decision := run.RetryDecision()
	if decision.Disposition != retrygovernance.DispositionAutomatic || decision.RemainingAutomaticAttempts != 0 || decision.ActionRequestID != "request-1" {
		t.Fatalf("authorized decision=%#v", decision)
	}
	if err := run.AuthorizeOneRetry(retrygovernance.AttemptOriginManual, "request-2", "event-2", now.Add(2*time.Second)); err == nil {
		t.Fatal("second authorization unexpectedly succeeded")
	}
}
