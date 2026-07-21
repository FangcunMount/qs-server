package attentionprojection

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

type stubSyncClient struct {
	err   error
	calls int
}

func (c *stubSyncClient) SyncAssessmentAttention(context.Context, uint64, string, bool) error {
	c.calls++
	return c.err
}

func TestProjectorRecordsFailureWhenRPCFails(t *testing.T) {
	store := NewMemoryStore()
	client := &stubSyncClient{err: errors.New("rpc unavailable")}
	projector := NewProjector(store, client, DefaultMaxAttempts, slog.New(slog.NewTextHandler(io.Discard, nil)))
	input := PendingInput{
		EventID: "evt-1", ReportID: "report-1", AssessmentID: "123",
		TesteeID: 99, RiskLevel: "severe", MarkKeyFocus: true,
	}

	if err := projector.Project(context.Background(), input); err != nil {
		t.Fatalf("Project returned error: %v", err)
	}
	rec, err := store.GetByEventID(context.Background(), input.EventID)
	if err != nil {
		t.Fatalf("GetByEventID: %v", err)
	}
	if rec.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", rec.Status)
	}
	if rec.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1", rec.Attempt)
	}
	if rec.LastError == "" {
		t.Fatal("expected last_error to be persisted")
	}
}

func TestProjectorDuplicateEventIsIdempotent(t *testing.T) {
	store := NewMemoryStore()
	client := &stubSyncClient{}
	projector := NewProjector(store, client, DefaultMaxAttempts, slog.New(slog.NewTextHandler(io.Discard, nil)))
	input := PendingInput{
		EventID: "evt-dup", ReportID: "report-1", AssessmentID: "123",
		TesteeID: 99, RiskLevel: "severe", MarkKeyFocus: true,
	}

	if err := projector.Project(context.Background(), input); err != nil {
		t.Fatalf("first Project: %v", err)
	}
	if err := projector.Project(context.Background(), input); err != nil {
		t.Fatalf("second Project: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("rpc calls = %d, want 1", client.calls)
	}
	rec, err := store.GetByEventID(context.Background(), input.EventID)
	if err != nil {
		t.Fatalf("GetByEventID: %v", err)
	}
	if rec.Status != StatusSucceeded {
		t.Fatalf("status = %q, want succeeded", rec.Status)
	}
}

func TestProjectorConvergesAfterReconcileRetry(t *testing.T) {
	store := NewMemoryStore()
	client := &stubSyncClient{err: errors.New("temporary outage")}
	projector := NewProjector(store, client, DefaultMaxAttempts, slog.New(slog.NewTextHandler(io.Discard, nil)))
	input := PendingInput{
		EventID: "evt-retry", ReportID: "report-1", AssessmentID: "123",
		TesteeID: 99, RiskLevel: "low", MarkKeyFocus: false,
	}

	if err := projector.Project(context.Background(), input); err != nil {
		t.Fatalf("Project: %v", err)
	}
	client.err = nil
	reconciler := NewReconciler(projector, 0, 10, slog.New(slog.NewTextHandler(io.Discard, nil)))
	reconciler.runOnce(context.Background())

	rec, err := store.GetByEventID(context.Background(), input.EventID)
	if err != nil {
		t.Fatalf("GetByEventID: %v", err)
	}
	if rec.Status != StatusSucceeded {
		t.Fatalf("status = %q, want succeeded", rec.Status)
	}
	if client.calls != 2 {
		t.Fatalf("rpc calls = %d, want 2", client.calls)
	}
}
