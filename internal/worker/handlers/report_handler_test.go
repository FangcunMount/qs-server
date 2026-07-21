package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/attentionprojection"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

type reportStatusWriterStub struct {
	completedAssessmentID        string
	completedReportID            string
	failedAssessmentID           string
	failedReason                 string
	temporarilyUnavailableID     string
	temporarilyUnavailableReason string
}

func (s *reportStatusWriterStub) SetProcessing(context.Context, string, string, string) {}

func (s *reportStatusWriterStub) SetCompleted(_ context.Context, assessmentID, _ string, reportID string) {
	s.completedAssessmentID = assessmentID
	s.completedReportID = reportID
}

func (s *reportStatusWriterStub) SetFailed(_ context.Context, assessmentID, _ string, reason, _ string) {
	s.failedAssessmentID = assessmentID
	s.failedReason = reason
}

func (s *reportStatusWriterStub) SetTemporarilyUnavailable(_ context.Context, assessmentID, _ string, reason, _ string) {
	s.temporarilyUnavailableID = assessmentID
	s.temporarilyUnavailableReason = reason
}

func TestHandleInterpretationReportGeneratedSyncsAssessmentAttentionForHighRisk(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	}
	handler := handleInterpretationReportGenerated(deps)

	if err := handler(context.Background(), eventcatalog.InterpretationReportGenerated, mustBuildReportGeneratedOutcomePayload(t, "high", "severe")); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.syncAssessmentAttentionCalls != 1 {
		t.Fatalf("expected one attention sync call, got %d", client.syncAssessmentAttentionCalls)
	}
	req := client.syncAssessmentAttentionRequest
	if req == nil {
		t.Fatal("expected attention sync request")
	}
	if req.TesteeId != 99 || req.RiskLevel != "severe" || !req.MarkKeyFocus {
		t.Fatalf("unexpected attention sync request: %#v", req)
	}
}

func TestHandleInterpretationReportGeneratedPersistsAttentionFailureWithoutFailingHandler(t *testing.T) {
	store := attentionprojection.NewMemoryStore()
	client := &fakeWorkerInternalClient{syncAssessmentAttentionErr: errors.New("rpc unavailable")}
	projector := attentionprojection.NewProjector(
		store,
		&internalAttentionSyncClient{client: client},
		attentionprojection.DefaultMaxAttempts,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	reporter := &reportStatusWriterStub{}
	handler := handleInterpretationReportGenerated(&Dependencies{
		Logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient:       client,
		AttentionProjector:   projector,
		ReportStatusReporter: reporter,
	})

	if err := handler(context.Background(), eventcatalog.InterpretationReportGenerated, mustBuildReportGeneratedOutcomePayload(t, "high", "severe")); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if reporter.completedAssessmentID != "123" {
		t.Fatalf("report status should still complete, got assessment=%q", reporter.completedAssessmentID)
	}
	rec, err := store.GetByEventID(context.Background(), "evt-report-generated-outcome")
	if err != nil {
		t.Fatalf("GetByEventID: %v", err)
	}
	if rec.Status != attentionprojection.StatusFailed {
		t.Fatalf("status = %q, want failed", rec.Status)
	}
}

func TestHandleInterpretationReportGeneratedAttentionProjectionIsIdempotent(t *testing.T) {
	store := attentionprojection.NewMemoryStore()
	client := &fakeWorkerInternalClient{}
	projector := attentionprojection.NewProjector(
		store,
		&internalAttentionSyncClient{client: client},
		attentionprojection.DefaultMaxAttempts,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	handler := handleInterpretationReportGenerated(&Dependencies{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient:     client,
		AttentionProjector: projector,
	})
	payload := mustBuildReportGeneratedOutcomePayload(t, "low", "low")

	if err := handler(context.Background(), eventcatalog.InterpretationReportGenerated, payload); err != nil {
		t.Fatalf("first handler: %v", err)
	}
	if err := handler(context.Background(), eventcatalog.InterpretationReportGenerated, payload); err != nil {
		t.Fatalf("second handler: %v", err)
	}
	if client.syncAssessmentAttentionCalls != 1 {
		t.Fatalf("rpc calls = %d, want 1", client.syncAssessmentAttentionCalls)
	}
	rec, err := store.GetByEventID(context.Background(), "evt-report-generated-outcome")
	if err != nil {
		t.Fatalf("GetByEventID: %v", err)
	}
	if rec.Status != attentionprojection.StatusSucceeded {
		t.Fatalf("status = %q, want succeeded", rec.Status)
	}
}

func TestHandleInterpretationReportGeneratedDoesNotAutoMarkLowerRisk(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	}
	handler := handleInterpretationReportGenerated(deps)

	if err := handler(context.Background(), eventcatalog.InterpretationReportGenerated, mustBuildReportGeneratedOutcomePayload(t, "low", "low")); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.syncAssessmentAttentionCalls != 1 {
		t.Fatalf("expected one attention sync call, got %d", client.syncAssessmentAttentionCalls)
	}
	if client.syncAssessmentAttentionRequest.MarkKeyFocus {
		t.Fatal("expected lower risk report not to request key focus mark")
	}
}

func TestHandleInterpretationReportGeneratedMarksReportStatusCompleted(t *testing.T) {
	reporter := &reportStatusWriterStub{}
	handler := handleInterpretationReportGenerated(&Dependencies{
		Logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReportStatusReporter: reporter,
	})

	if err := handler(context.Background(), eventcatalog.InterpretationReportGenerated, mustBuildReportGeneratedOutcomePayload(t, "low", "low")); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if reporter.completedAssessmentID != "123" || reporter.completedReportID != "report-1" {
		t.Fatalf("completed status = assessment:%q report:%q", reporter.completedAssessmentID, reporter.completedReportID)
	}
}

// A failed report is an auditable Interpretation fact, not a command to run
// Interpretation again. Retrying is owned by delivery/retry policy or an
// explicit command, never by the failed-event consumer.
func TestHandleInterpretationReportFailedDoesNotTriggerReportGeneration(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	}
	handler := handleInterpretationReportFailed(deps)

	if err := handler(context.Background(), eventcatalog.InterpretationReportFailed, mustBuildReportFailedPayload(t, true, "automatic")); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if client.generateReportCalls != 0 {
		t.Fatalf("failed report event retriggered report generation: calls=%d", client.generateReportCalls)
	}
}

func TestHandleInterpretationReportFailedMarksTerminalStatus(t *testing.T) {
	reporter := &reportStatusWriterStub{}
	handler := handleInterpretationReportFailed(&Dependencies{
		Logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReportStatusReporter: reporter,
	})

	if err := handler(context.Background(), eventcatalog.InterpretationReportFailed, mustBuildReportFailedPayload(t, false, "terminal")); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if reporter.failedAssessmentID != "123" || reporter.failedReason != "interpretation_report_failed" {
		t.Fatalf("failed status = assessment:%q reason:%q", reporter.failedAssessmentID, reporter.failedReason)
	}
}

func TestHandleInterpretationReportFailedProjectsManualRequiredAsTemporarilyUnavailable(t *testing.T) {
	reporter := &reportStatusWriterStub{}
	handler := handleInterpretationReportFailed(&Dependencies{
		Logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReportStatusReporter: reporter,
	})

	if err := handler(context.Background(), eventcatalog.InterpretationReportFailed, mustBuildReportFailedPayload(t, true, "manual_required")); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if reporter.temporarilyUnavailableID != "123" || reporter.temporarilyUnavailableReason != "waiting_manual_action" {
		t.Fatalf("manual projection = assessment:%q reason:%q", reporter.temporarilyUnavailableID, reporter.temporarilyUnavailableReason)
	}
	if reporter.failedAssessmentID != "" {
		t.Fatalf("manual_required must not mark failed, got reason=%q", reporter.failedReason)
	}
}

func TestHandleInterpretationReportFailedKeepsAutomaticInFlight(t *testing.T) {
	reporter := &reportStatusWriterStub{}
	handler := handleInterpretationReportFailed(&Dependencies{
		Logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReportStatusReporter: reporter,
	})

	if err := handler(context.Background(), eventcatalog.InterpretationReportFailed, mustBuildReportFailedPayload(t, true, "automatic")); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if reporter.failedAssessmentID != "" || reporter.temporarilyUnavailableID != "" {
		t.Fatalf("automatic retry must keep patient projection in-flight, failed=%q unavailable=%q", reporter.failedAssessmentID, reporter.temporarilyUnavailableID)
	}
}

func mustBuildReportGeneratedOutcomePayload(t *testing.T, severity, levelCode string) []byte {
	t.Helper()

	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"id":            "evt-report-generated-outcome",
		"eventType":     eventcatalog.InterpretationReportGenerated,
		"occurredAt":    now,
		"aggregateType": "ReportGeneration",
		"aggregateID":   "generation-1",
		"data": map[string]any{
			"org_id":                 18,
			"generation_id":          "generation-1",
			"run_id":                 "run-1",
			"report_id":              "report-1",
			"assessment_id":          "123",
			"outcome_id":             "9001",
			"testee_id":              99,
			"attempt":                1,
			"report_type":            "standard",
			"template_version":       "v2",
			"builder_identity":       "factor-scoring",
			"content_schema_version": "report-content/v2",
			"model": map[string]any{
				"kind":      "scale",
				"algorithm": "scale_default",
				"code":      "SDS",
			},
			"primary_score": map[string]any{"kind": "raw_total", "value": 42.0},
			"level":         map[string]any{"code": levelCode, "label": levelCode, "severity": severity},
			"generated_at":  now,
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}

func mustBuildReportFailedPayload(t *testing.T, retryable bool, disposition string) []byte {
	t.Helper()

	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	data := map[string]any{
		"org_id":           18,
		"generation_id":    "generation-1",
		"run_id":           "run-2",
		"assessment_id":    "123",
		"outcome_id":       "9001",
		"testee_id":        99,
		"attempt":          2,
		"report_type":      "standard",
		"template_version": "v2",
		"failure_kind":     "template",
		"failure_code":     "not_found",
		"retryable":        retryable,
		"safe_reason":      "template unavailable",
		"failed_at":        now,
	}
	if disposition != "" {
		data["retry_decision"] = map[string]any{
			"disposition": disposition,
			"retryable":   retryable,
			"attempt":     2,
		}
	}
	payload, err := json.Marshal(map[string]any{
		"id":            "evt-report-failed",
		"eventType":     eventcatalog.InterpretationReportFailed,
		"occurredAt":    now,
		"aggregateType": "ReportGeneration",
		"aggregateID":   "generation-1",
		"data":          data,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}
