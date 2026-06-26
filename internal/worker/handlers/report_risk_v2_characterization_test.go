package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestV2ReportGeneratedHighSeverityMarksKeyFocus(t *testing.T) {
	tests := []struct {
		name         string
		severity     string
		levelCode    string
		wantKeyFocus bool
		wantRisk     string
	}{
		{name: "severity high", severity: "high", levelCode: "severe", wantKeyFocus: true, wantRisk: "severe"},
		{name: "severity medium", severity: "medium", levelCode: "medium", wantKeyFocus: false, wantRisk: "medium"},
		{name: "typology none", severity: "none", levelCode: "INTJ", wantKeyFocus: false, wantRisk: "none"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeWorkerInternalClient{}
			deps := &Dependencies{
				Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
				InternalClient: client,
			}
			handler := handleReportGenerated(deps)
			if err := handler(context.Background(), "report.generated.v2", mustBuildReportGeneratedV2Payload(t, tc.severity, tc.levelCode)); err != nil {
				t.Fatalf("handler: %v", err)
			}
			req := client.syncAssessmentAttentionRequest
			if req == nil {
				t.Fatal("expected attention sync request")
			}
			if req.MarkKeyFocus != tc.wantKeyFocus || req.RiskLevel != tc.wantRisk {
				t.Fatalf("request = %#v, want MarkKeyFocus=%v risk=%s", req, tc.wantKeyFocus, tc.wantRisk)
			}
		})
	}
}

func mustBuildReportGeneratedV2Payload(t *testing.T, severity, levelCode string) []byte {
	t.Helper()
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"id":            "evt-report-generated-v2",
		"eventType":     "report.generated.v2",
		"occurredAt":    now,
		"aggregateType": "Report",
		"aggregateID":   "report-1",
		"data": map[string]any{
			"report_id":     "report-1",
			"assessment_id": "123",
			"testee_id":     99,
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
