package handlers

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

// V1 contract: report.generated with risk_level severe/high marks key focus;
// lower risk levels do not.
func TestV1ReportGeneratedHighRiskMarksKeyFocus(t *testing.T) {
	tests := []struct {
		riskLevel    string
		wantKeyFocus bool
	}{
		{riskLevel: "severe", wantKeyFocus: true},
		{riskLevel: "high", wantKeyFocus: true},
		{riskLevel: "medium", wantKeyFocus: false},
		{riskLevel: "low", wantKeyFocus: false},
		{riskLevel: "none", wantKeyFocus: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.riskLevel, func(t *testing.T) {
			client := &fakeWorkerInternalClient{}
			deps := &Dependencies{
				Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
				InternalClient: client,
			}
			handler := handleReportGenerated(deps)

			if err := handler(context.Background(), "report.generated", mustBuildReportGeneratedPayload(t, tc.riskLevel)); err != nil {
				t.Fatalf("handler: %v", err)
			}
			if client.syncAssessmentAttentionCalls != 1 {
				t.Fatalf("attention sync calls = %d, want 1", client.syncAssessmentAttentionCalls)
			}
			req := client.syncAssessmentAttentionRequest
			if req == nil {
				t.Fatal("expected attention sync request")
			}
			if req.TesteeId != 99 || req.RiskLevel != tc.riskLevel || req.MarkKeyFocus != tc.wantKeyFocus {
				t.Fatalf("request = %#v, want MarkKeyFocus=%v risk=%s", req, tc.wantKeyFocus, tc.riskLevel)
			}
		})
	}
}
