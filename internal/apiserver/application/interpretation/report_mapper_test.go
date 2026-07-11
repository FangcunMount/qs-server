package interpretation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

func TestReportRowToResultMapsLegacyReportFields(t *testing.T) {
	createdAt := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	got := reportRowToResult(evaluationreadmodel.ReportRow{
		AssessmentID: 7001, ModelName: "抑郁自评量表", ModelCode: "SDS",
		TotalScore: 72, RiskLevel: "medium", Conclusion: "中度", CreatedAt: createdAt,
	})
	if got == nil || got.ModelName != "抑郁自评量表" || got.ModelCode != "SDS" || !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("report result = %#v", got)
	}
}

func TestReportRowToOutcomeResultPrefersExplicitModelProjection(t *testing.T) {
	row := evaluationreadmodel.ReportRow{
		AssessmentID: 202,
		Model: evaluationreadmodel.ModelIdentityRow{
			Kind: "typology", SubKind: "typology", Algorithm: "mbti", Code: "MBTI-16P", Title: "MBTI",
			ProductChannel: "behavior_ability", AlgorithmFamily: "typology",
		},
		PrimaryScore: &evaluationreadmodel.ScoreValueRow{Kind: "match_percent", Value: 88, Label: "88%"},
		Level:        &evaluationreadmodel.ResultLevelRow{Code: "INTJ", Label: "INTJ", Severity: "none"},
		Conclusion:   "建筑师", CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	result := reportRowToOutcomeResult(row)
	if result.Model.Kind != "typology" || result.Model.SubKind != "typology" || result.Model.ProductChannel != "behavior_ability" || result.Model.AlgorithmFamily == "" {
		t.Fatalf("unexpected model: %#v", result.Model)
	}
	if result.PrimaryScore == nil || result.PrimaryScore.Kind != "match_percent" || result.Level == nil || result.Level.Code != "INTJ" {
		t.Fatalf("unexpected outcome summary: %#v", result)
	}
}
