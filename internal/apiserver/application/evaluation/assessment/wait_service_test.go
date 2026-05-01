package assessment

import (
	"testing"
)

func TestAssessmentStatusSummary(t *testing.T) {
	score := 12.5
	riskLevel := "high"

	summary, done := assessmentStatusSummary(&AssessmentResult{
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

	pendingSummary, pendingDone := assessmentStatusSummary(&AssessmentResult{Status: "submitted"})
	if pendingDone {
		t.Fatal("pending done = true, want false")
	}
	if pendingSummary.Status != "" {
		t.Fatalf("pending summary status = %q, want empty", pendingSummary.Status)
	}
}

func TestBuildAssessmentStatusSummaryCopiesValues(t *testing.T) {
	t.Parallel()

	score := 18.5
	riskLevel := "medium"
	summary := buildAssessmentStatusSummary(&AssessmentResult{
		Status:     "failed",
		TotalScore: &score,
		RiskLevel:  &riskLevel,
	})

	score = 99
	riskLevel = "changed"

	if summary.Status != "failed" {
		t.Fatalf("status = %q, want failed", summary.Status)
	}
	if summary.TotalScore == nil || *summary.TotalScore != 18.5 {
		t.Fatalf("total_score = %v, want 18.5", summary.TotalScore)
	}
	if summary.RiskLevel == nil || *summary.RiskLevel != "medium" {
		t.Fatalf("risk_level = %v, want medium", summary.RiskLevel)
	}
}

func TestPendingAssessmentStatusSummary(t *testing.T) {
	t.Parallel()

	summary := pendingAssessmentStatusSummary()
	if summary.Status != "pending" {
		t.Fatalf("status = %q, want pending", summary.Status)
	}
	if summary.UpdatedAt <= 0 {
		t.Fatalf("updated_at = %d, want positive unix timestamp", summary.UpdatedAt)
	}
}
