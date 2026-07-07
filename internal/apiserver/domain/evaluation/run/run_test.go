package run

import (
	"testing"
	"time"
)

func TestNextEvaluationRunIncrementsAttempt(t *testing.T) {
	t.Parallel()

	first := NewEvaluationRunWithAttempt(42, 1)
	first.Fail(time.Now(), Failure{Kind: FailureKindTimeout, Message: "timed out", Retryable: true})
	second := NextEvaluationRun(first)
	if second.Attempt.Number != 2 {
		t.Fatalf("attempt=%d, want 2", second.Attempt.Number)
	}
	if second.RunID != "42:2" {
		t.Fatalf("run id=%s", second.RunID)
	}
}

func TestEvaluationRunLifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	run := NewEvaluationRun(42)
	run.Start(now)
	if run.Attempt.Status != StatusRunning {
		t.Fatalf("status=%s", run.Attempt.Status)
	}

	done := now.Add(time.Minute)
	run.Succeed(done)
	if run.Attempt.Status != StatusSucceeded {
		t.Fatalf("status=%s", run.Attempt.Status)
	}
	if run.FinishedAt == nil || !run.FinishedAt.Equal(done) {
		t.Fatalf("finishedAt=%v", run.FinishedAt)
	}
}

func TestEvaluationRunFailureRetryable(t *testing.T) {
	t.Parallel()

	run := NewEvaluationRun(7)
	run.Fail(time.Now(), Failure{Kind: FailureKindTimeout, Message: "timed out", Retryable: true})
	if !run.Retryable() {
		t.Fatal("expected retryable failure")
	}
}
