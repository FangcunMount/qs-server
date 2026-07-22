package response

import (
	"testing"
	"time"

	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
)

func TestNewAssessmentResponseAddsLabelsAndFormatsTimes(t *testing.T) {
	submittedAt := time.Date(2026, 4, 17, 13, 25, 27, 0, time.Local)
	riskLevel := "high"
	result := &evaluationoperator.Assessment{
		ID:                   1,
		OrgID:                2,
		TesteeID:             3,
		QuestionnaireCode:    "q",
		QuestionnaireVersion: "v1",
		AnswerSheetID:        4,
		OriginType:           "plan",
		Status:               "evaluated",
		RiskLevel:            &riskLevel,
		SubmittedAt:          &submittedAt,
	}

	resp := NewAssessmentResponse(result)
	if resp == nil {
		t.Fatal("expected response")
		return
	}
	if resp.OriginTypeLabel != "计划测评" {
		t.Fatalf("origin_type_label = %q, want %q", resp.OriginTypeLabel, "计划测评")
	}
	if resp.StatusLabel != "已计分" {
		t.Fatalf("status_label = %q, want %q", resp.StatusLabel, "已计分")
	}
	if resp.RiskLevelLabel != "高风险" {
		t.Fatalf("risk_level_label = %q, want %q", resp.RiskLevelLabel, "高风险")
	}
	if resp.SubmittedAt == nil || *resp.SubmittedAt != "2026-04-17 13:25:27" {
		t.Fatalf("submitted_at = %#v, want %q", resp.SubmittedAt, "2026-04-17 13:25:27")
	}
}
