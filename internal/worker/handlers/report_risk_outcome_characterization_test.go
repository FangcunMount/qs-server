package handlers

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

func TestOutcomeReportGeneratedHighSeverityMarksKeyFocus(t *testing.T) {
	tests := []struct {
		name         string
		severity     string
		levelCode    string
		wantKeyFocus bool
		wantRisk     string
	}{
		{name: "severity severe", severity: "severe", levelCode: "severe", wantKeyFocus: true, wantRisk: "severe"},
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
			handler := handleInterpretationReportGenerated(deps)
			if err := handler(context.Background(), eventcatalog.InterpretationReportGenerated, mustBuildReportGeneratedOutcomePayload(t, tc.severity, tc.levelCode)); err != nil {
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
