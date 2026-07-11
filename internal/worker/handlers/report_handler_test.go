package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

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

func mustBuildReportGeneratedOutcomePayload(t *testing.T, severity, levelCode string) []byte {
	t.Helper()

	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"id":            "evt-report-generated-outcome",
		"eventType":     eventcatalog.InterpretationReportGenerated,
		"occurredAt":    now,
		"aggregateType": "Report",
		"aggregateID":   "report-1",
		"data": map[string]any{
			"org_id":        18,
			"report_id":     "report-1",
			"assessment_id": "123",
			"outcome_id":    "9001",
			"testee_id":     99,
			"attempt":       1,
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
