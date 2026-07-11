package run

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
