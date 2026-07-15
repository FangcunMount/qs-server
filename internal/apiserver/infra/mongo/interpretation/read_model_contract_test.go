package interpretation

import (
	"testing"
	"time"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestReportPOToReadRowMapsReportDocumentShape(t *testing.T) {
	maxScore := 100.0
	factorCode := "sleep"
	createdAt := time.Date(2026, 5, 2, 11, 0, 0, 0, time.UTC)

	row := projectArchivedReportRow(&ArchivedReportPO{
		BaseDocument: base.BaseDocument{
			DomainID:  meta.FromUint64(7001),
			CreatedAt: createdAt,
		},
		ScaleName:  "SDS",
		ScaleCode:  "sds",
		TesteeID:   8001,
		TotalScore: 88,
		RiskLevel:  "high",
		Conclusion: "高风险",
		Dimensions: []DimensionInterpretPO{
			{
				FactorCode:    "total",
				FactorName:    "总分",
				RawScore:      88,
				MaxScore:      &maxScore,
				RiskLevel:     "high",
				DerivedScores: []ScoreValuePO{{Kind: "t_score", Value: 65}, {Kind: "percentile", Value: 90}},
				Level:         &ResultLevelPO{Code: "elevated", Label: "偏高", Severity: "high"},
				NormReference: &NormReferencePO{ScoreKind: "t_score", Benchmark: 50, TableVersion: "2026", MinAgeMonths: 60, MaxAgeMonths: 95},
				Description:   "高风险描述",
				Suggestion:    "建议干预",
			},
		},
		Suggestions: []SuggestionPO{
			{Category: "general", Content: "总体建议"},
			{Category: "dimension", Content: "睡眠建议", FactorCode: &factorCode},
		},
	})

	if row.AssessmentID != 7001 || row.ModelName != "SDS" || row.ModelCode != "sds" {
		t.Fatalf("unexpected report identity: %#v", row)
	}
	if row.TotalScore != 88 || row.RiskLevel != "high" || row.Conclusion != "高风险" || !row.CreatedAt.Equal(createdAt) {
		t.Fatalf("unexpected report summary: %#v", row)
	}
	if row.Model.Kind != "scale" || row.Model.Code != "sds" || row.Model.Title != "SDS" || row.Model.ProductChannel == "" || row.Model.AlgorithmFamily == "" {
		t.Fatalf("legacy model identity was not normalized: %#v", row.Model)
	}
	if row.PrimaryScore == nil || row.PrimaryScore.Kind != "raw_total" || row.PrimaryScore.Value != 88 {
		t.Fatalf("legacy primary score was not normalized: %#v", row.PrimaryScore)
	}
	if row.Level == nil || row.Level.Code != "high" || row.Level.Severity != "high" {
		t.Fatalf("legacy result level was not normalized: %#v", row.Level)
	}
	if len(row.Dimensions) != 1 || row.Dimensions[0].FactorCode != "total" || row.Dimensions[0].MaxScore == nil || *row.Dimensions[0].MaxScore != maxScore {
		t.Fatalf("unexpected dimensions: %#v", row.Dimensions)
	}
	if len(row.Dimensions[0].DerivedScores) != 2 || row.Dimensions[0].Level == nil || row.Dimensions[0].Level.Code != "elevated" || row.Dimensions[0].NormReference == nil || row.Dimensions[0].NormReference.Benchmark != 50 {
		t.Fatalf("unexpected dimension score context: %#v", row.Dimensions[0])
	}
	if len(row.Suggestions) != 2 || row.Suggestions[1].FactorCode == nil || *row.Suggestions[1].FactorCode != factorCode {
		t.Fatalf("unexpected suggestions: %#v", row.Suggestions)
	}
}

func TestReportPOToReadRowToleratesNilLegacySlices(t *testing.T) {
	row := projectArchivedReportRow(&ArchivedReportPO{
		BaseDocument: base.BaseDocument{DomainID: meta.FromUint64(7001)},
	})

	if row.AssessmentID != 7001 {
		t.Fatalf("assessment id = %d, want 7001", row.AssessmentID)
	}
	if row.Dimensions == nil {
		t.Fatalf("dimensions should be an empty slice for stable response mapping")
	}
	if row.Suggestions == nil {
		t.Fatalf("suggestions should be an empty slice for stable response mapping")
	}
}
