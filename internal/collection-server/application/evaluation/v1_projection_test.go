package evaluation

import "testing"

func TestDetailToLegacyScaleProjection(t *testing.T) {
	t.Parallel()

	got := DetailToLegacy(&AssessmentDetailResponse{
		ID:       "1",
		OrgID:    "2",
		TesteeID: "3",
		Model: ModelIdentityResponse{
			Kind:  "scale",
			Code:  "PHQ9",
			Title: "抑郁筛查",
		},
		PrimaryScore: &ScoreValueResponse{Value: 8},
		Level:        &ResultLevelResponse{Code: "low", Label: "轻度"},
		Status:       "interpreted",
	})
	if got.ScaleCode != "PHQ9" || got.TotalScore != 8 || got.RiskLevel != "low" {
		t.Fatalf("detail = %#v", got)
	}
}

func TestDetailToLegacyPersonalityOmitsScaleFields(t *testing.T) {
	t.Parallel()

	got := DetailToLegacy(&AssessmentDetailResponse{
		Model: ModelIdentityResponse{Kind: personalityModelKind, Code: "MBTI"},
		Level: &ResultLevelResponse{Code: "INTJ", Label: "INTJ"},
	})
	if got.ScaleCode != "" || got.RiskLevel != "" {
		t.Fatalf("personality detail = %#v", got)
	}
}

func TestReportToLegacyProjection(t *testing.T) {
	t.Parallel()

	got := ReportToLegacy(&AssessmentReportResponse{
		AssessmentID: "9",
		Model:        ModelIdentityResponse{Kind: "scale", Code: "PHQ9", Title: "抑郁筛查"},
		PrimaryScore: &ScoreValueResponse{Value: 12},
		Level:        &ResultLevelResponse{Code: "medium"},
		Conclusion:   "中度",
	})
	if got.ScaleCode != "PHQ9" || got.TotalScore != 12 || got.RiskLevel != "medium" {
		t.Fatalf("report = %#v", got)
	}
}
