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
	reconciler, err := NewFactReconciler(
		factSourceStub{facts: []ReportFact{{ReportID: "1", AssessmentID: "2", TesteeID: 3, RiskLevel: "high", MarkKeyFocus: true}}},
		store, projector, time.Now().Add(-time.Hour), true, 0, 500, nil,
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
	reconciler, err := NewFactReconciler(
		factSourceStub{facts: []ReportFact{{ReportID: "1", AssessmentID: "2", TesteeID: 3, RiskLevel: "severe", MarkKeyFocus: true}}},
		store, projector, time.Now().Add(-time.Hour), false, 0, 500, nil,
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
