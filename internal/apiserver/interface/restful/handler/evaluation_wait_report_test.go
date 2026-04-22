package handler

import (
	"testing"
	"time"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
)

func TestParseWaitReportTimeout(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{name: "uses valid timeout", raw: "30", want: 30 * time.Second},
		{name: "falls back on invalid timeout", raw: "bad", want: 15 * time.Second},
		{name: "falls back on too small timeout", raw: "3", want: 15 * time.Second},
		{name: "falls back on too large timeout", raw: "90", want: 15 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseWaitReportTimeout(tt.raw); got != tt.want {
				t.Fatalf("parseWaitReportTimeout(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestAssessmentStatusSummary(t *testing.T) {
	score := 12.5
	riskLevel := "high"

	summary, done := assessmentStatusSummary(&assessmentApp.AssessmentResult{
		Status:     "interpreted",
		TotalScore: &score,
		RiskLevel:  &riskLevel,
	})
	if !done {
		t.Fatal("done = false, want true")
	}
	if summary.Status != "interpreted" {
		t.Fatalf("status = %q, want interpreted", summary.Status)
	}
	if summary.TotalScore == nil || *summary.TotalScore != score {
		t.Fatalf("total_score = %v, want %v", summary.TotalScore, score)
	}
	if summary.RiskLevel == nil || *summary.RiskLevel != riskLevel {
		t.Fatalf("risk_level = %v, want %v", summary.RiskLevel, riskLevel)
	}

	pendingSummary, pendingDone := assessmentStatusSummary(&assessmentApp.AssessmentResult{Status: "submitted"})
	if pendingDone {
		t.Fatal("pending done = true, want false")
	}
	if pendingSummary.Status != "" {
		t.Fatalf("pending summary status = %q, want empty", pendingSummary.Status)
	}
}
