package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestHandleReportGeneratedSyncsAssessmentAttentionForHighRisk(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	}
	handler := handleReportGenerated(deps)

	if err := handler(context.Background(), "report.generated", mustBuildReportGeneratedPayload(t, "severe")); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.syncAssessmentAttentionCalls != 1 {
		t.Fatalf("expected one attention sync call, got %d", client.syncAssessmentAttentionCalls)
	}
	req := client.syncAssessmentAttentionRequest
	if req == nil {
		t.Fatalf("expected attention sync request")
	}
	if req.TesteeId != 99 || req.RiskLevel != "severe" || !req.MarkKeyFocus {
		t.Fatalf("unexpected attention sync request: %#v", req)
	}
}

func TestHandleReportGeneratedDoesNotAutoMarkLowerRisk(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	}
	handler := handleReportGenerated(deps)

	if err := handler(context.Background(), "report.generated", mustBuildReportGeneratedPayload(t, "low")); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.syncAssessmentAttentionCalls != 1 {
		t.Fatalf("expected one attention sync call, got %d", client.syncAssessmentAttentionCalls)
	}
	if client.syncAssessmentAttentionRequest.MarkKeyFocus {
		t.Fatalf("expected lower risk report not to request key focus mark")
	}
}

func mustBuildReportGeneratedPayload(t *testing.T, riskLevel string) []byte {
	t.Helper()

	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"id":            "evt-report-generated",
		"eventType":     "report.generated",
		"occurredAt":    now,
		"aggregateType": "Report",
		"aggregateID":   "report-1",
		"data": map[string]any{
			"report_id":     "report-1",
			"assessment_id": "123",
			"testee_id":     99,
			"scale_code":    "scale-1",
			"scale_version": "1.0.0",
			"total_score":   42,
			"risk_level":    riskLevel,
			"generated_at":  now,
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}
