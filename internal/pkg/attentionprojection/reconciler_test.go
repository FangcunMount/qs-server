package attentionprojection

import (
	"context"
	"testing"
)

func TestReconcilerRunsOnlyAsAttentionLeader(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	input := PendingInput{EventID: "evt-1", ReportID: "report-1", AssessmentID: "assessment-1", TesteeID: 9, RiskLevel: "high", MarkKeyFocus: true}
	if _, err := store.EnsurePending(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	client := &syncClientStub{}
	projector := NewProjector(store, client, DefaultMaxAttempts, nil)
	runner := &reconcileRunnerStub{}
	reconciler, err := NewReconciler(projector, runner, 0, 100, nil)
	if err != nil {
		t.Fatal(err)
	}

	acquired, err := reconciler.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acquired || client.calls != 0 {
		t.Fatalf("contention round acquired=%v calls=%d, want skipped", acquired, client.calls)
	}

	runner.acquired = true
	acquired, err = reconciler.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acquired || client.calls != 1 {
		t.Fatalf("leader round acquired=%v calls=%d, want one sync", acquired, client.calls)
	}
	if runner.workload != "attention_projection_reconcile" || runner.key != reconcileLeaseKey {
		t.Fatalf("lease identity = %q/%q", runner.workload, runner.key)
	}
}

func TestReconcilerRequiresLeaseRunner(t *testing.T) {
	t.Parallel()

	if _, err := NewReconciler(NewProjector(NewMemoryStore(), &syncClientStub{}, DefaultMaxAttempts, nil), nil, 0, 100, nil); err == nil {
		t.Fatal("NewReconciler() error = nil, want missing lease runner rejection")
	}
}
