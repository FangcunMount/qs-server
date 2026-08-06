package attentionprojection

import (
	"context"
	"testing"
	"time"
)

type factSourceStub struct {
	facts []ReportFact
}

func (s factSourceStub) ListReportFacts(context.Context, time.Time, string, int) ([]ReportFact, string, error) {
	return append([]ReportFact(nil), s.facts...), "", nil
}

type syncClientStub struct{ calls int }

func (s *syncClientStub) SyncAssessmentAttention(context.Context, uint64, string, bool) error {
	s.calls++
	return nil
}

func TestFactReconcilerDryRunDoesNotCreateProjection(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	client := &syncClientStub{}
	projector := NewProjector(store, client, DefaultMaxAttempts, nil)
	runner := &reconcileRunnerStub{acquired: true}
	reconciler, err := NewFactReconciler(
		factSourceStub{facts: []ReportFact{{ReportID: "1", AssessmentID: "2", TesteeID: 3, RiskLevel: "high", MarkKeyFocus: true}}},
		store, projector, runner, time.Now().Add(-time.Hour), true, 0, 500, FactManifestGuard{}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reconciler.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Missing != 1 || result.Created != 0 || client.calls != 0 {
		t.Fatalf("result=%#v calls=%d", result, client.calls)
	}
}

func TestFactReconcilerCreatesOnlyMissingProjection(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	client := &syncClientStub{}
	projector := NewProjector(store, client, DefaultMaxAttempts, nil)
	runner := &reconcileRunnerStub{acquired: true}
	reconciler, err := NewFactReconciler(
		factSourceStub{facts: []ReportFact{{ReportID: "1", AssessmentID: "2", TesteeID: 3, RiskLevel: "severe", MarkKeyFocus: true}}},
		store, projector, runner, time.Now().Add(-time.Hour), false, 0, 500, FactManifestGuard{}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reconciler.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || client.calls != 1 {
		t.Fatalf("result=%#v calls=%d", result, client.calls)
	}
	record, err := store.FindByReportID(context.Background(), "1")
	if err != nil || record.Status != StatusSucceeded {
		t.Fatalf("record=%#v err=%v", record, err)
	}
}

func TestFactReconcilerSkipsWhenAnotherWorkerIsLeader(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	client := &syncClientStub{}
	projector := NewProjector(store, client, DefaultMaxAttempts, nil)
	runner := &reconcileRunnerStub{}
	reconciler, err := NewFactReconciler(
		factSourceStub{facts: []ReportFact{{ReportID: "1", AssessmentID: "2", TesteeID: 3, RiskLevel: "severe", MarkKeyFocus: true}}},
		store, projector, runner, time.Now().Add(-time.Hour), false, 0, 500, FactManifestGuard{}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reconciler.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result != (FactReconcileResult{}) || client.calls != 0 {
		t.Fatalf("contention result=%#v calls=%d, want skipped", result, client.calls)
	}
	if runner.workload != "attention_projection_reconcile" || runner.key != reconcileLeaseKey {
		t.Fatalf("lease identity = %q/%q", runner.workload, runner.key)
	}
}

func TestFactReconcilerTargetManifestCreatesOnlyAllowlistedReports(t *testing.T) {
	t.Parallel()

	target := ReportFact{ReportID: "11", AssessmentID: "21", TesteeID: 31, RiskLevel: "high", MarkKeyFocus: true}
	unlisted := ReportFact{ReportID: "12", AssessmentID: "22", TesteeID: 32, RiskLevel: "severe", MarkKeyFocus: true}
	fingerprint, err := factManifestFingerprint([]ReportFact{target})
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemoryStore()
	client := &syncClientStub{}
	reconciler, err := NewFactReconciler(
		factSourceStub{facts: []ReportFact{unlisted, target}}, store,
		NewProjector(store, client, DefaultMaxAttempts, nil), &reconcileRunnerStub{acquired: true},
		time.Now().Add(-time.Hour), false, 0, 500,
		FactManifestGuard{ReportIDs: []string{target.ReportID}, Fingerprint: fingerprint}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reconciler.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 1 || result.Created != 1 || client.calls != 1 {
		t.Fatalf("result=%#v calls=%d", result, client.calls)
	}
	if _, err := store.FindByReportID(context.Background(), unlisted.ReportID); err == nil {
		t.Fatal("unlisted report was projected")
	}
}

func TestFactReconcilerTargetManifestRejectsFingerprintBeforeWrites(t *testing.T) {
	t.Parallel()

	fact := ReportFact{ReportID: "11", AssessmentID: "21", TesteeID: 31, RiskLevel: "high", MarkKeyFocus: true}
	store := NewMemoryStore()
	client := &syncClientStub{}
	reconciler, err := NewFactReconciler(
		factSourceStub{facts: []ReportFact{fact}}, store,
		NewProjector(store, client, DefaultMaxAttempts, nil), &reconcileRunnerStub{acquired: true},
		time.Now().Add(-time.Hour), false, 0, 500,
		FactManifestGuard{ReportIDs: []string{fact.ReportID}, Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.RunOnce(context.Background()); err == nil {
		t.Fatal("fingerprint mismatch was accepted")
	}
	if client.calls != 0 {
		t.Fatalf("fingerprint mismatch performed %d writes", client.calls)
	}
}

func TestFactReconcilerTargetManifestIsResumableAndIdempotent(t *testing.T) {
	t.Parallel()

	fact := ReportFact{ReportID: "11", AssessmentID: "21", TesteeID: 31, RiskLevel: "severe", MarkKeyFocus: true}
	fingerprint, err := factManifestFingerprint([]ReportFact{fact})
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemoryStore()
	client := &syncClientStub{}
	reconciler, err := NewFactReconciler(
		factSourceStub{facts: []ReportFact{fact}}, store,
		NewProjector(store, client, DefaultMaxAttempts, nil), &reconcileRunnerStub{acquired: true},
		time.Now().Add(-time.Hour), false, 0, 500,
		FactManifestGuard{ReportIDs: []string{fact.ReportID}, Fingerprint: fingerprint}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	first, err := reconciler.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := reconciler.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Created != 1 || second.Created != 0 || second.Existing != 1 || client.calls != 1 {
		t.Fatalf("first=%#v second=%#v calls=%d", first, second, client.calls)
	}
}
