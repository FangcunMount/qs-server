package characterization_test

import (
	"testing"
	"time"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	mongoevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
)

// V1 contract: Mongo report mapper preserves scale summary, factor dimensions,
// suggestions, and typology model extra across round trip.
func TestV1MongoReportMapperPreservesScaleAndPersonalityFields(t *testing.T) {
	mapper := mongoevaluation.NewReportMapper()
	createdAt := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	maxScore := 27.0

	original := domainreport.ReconstructInterpretReport(
		domainreport.ID(42),
		"抑郁筛查",
		"PHQ9",
		8,
		domainreport.RiskLevelLow,
		"总分提示轻度风险",
		[]domainreport.DimensionInterpret{
			domainreport.NewDimensionInterpret(
				domainreport.NewFactorCode("TOTAL"),
				"总分",
				8,
				&maxScore,
				domainreport.RiskLevelLow,
				"总分提示轻度风险",
				"保持规律作息",
			),
		},
		[]domainreport.Suggestion{
			{Category: domainreport.SuggestionCategoryGeneral, Content: "持续观察整体状态"},
			{Category: domainreport.SuggestionCategoryGeneral, Content: "保持规律作息"},
		},
		nil,
		createdAt,
		&updatedAt,
	)

	got := mapper.ToDomain(mapper.ToPO(original, 9))
	if got.ID() != original.ID() ||
		got.ModelName() != original.ModelName() ||
		got.ModelCode() != original.ModelCode() ||
		got.TotalScore() != original.TotalScore() ||
		got.RiskLevel() != original.RiskLevel() ||
		got.Conclusion() != original.Conclusion() {
		t.Fatalf("scale round trip summary mismatch: got=%#v", got)
	}
	if len(got.Dimensions()) != 1 || got.Dimensions()[0].Code().String() != "TOTAL" {
		t.Fatalf("dimensions = %#v", got.Dimensions())
	}
	if len(got.Suggestions()) != 2 {
		t.Fatalf("suggestions = %#v", got.Suggestions())
	}

	personality := domainreport.ReconstructInterpretReport(
		domainreport.ID(43),
		"MBTI 人格类型测评",
		"MBTI_OEJTS",
		0,
		domainreport.RiskLevelNone,
		"善于长远规划",
		nil,
		[]domainreport.Suggestion{
			{Category: domainreport.SuggestionCategoryGeneral, Content: "建议：保留沟通空间"},
		},
		&domainreport.ModelExtra{
			Kind:         "mbti",
			TypeCode:     "INTJ",
			TypeName:     "建筑师",
			MatchPercent: 75,
		},
		createdAt,
		nil,
	)
	gotPersonality := mapper.ToDomain(mapper.ToPO(personality, 10))
	if gotPersonality.ModelExtra() == nil ||
		gotPersonality.ModelExtra().TypeCode != "INTJ" ||
		gotPersonality.ModelExtra().MatchPercent != 75 {
		t.Fatalf("personality round trip extra = %#v", gotPersonality.ModelExtra())
	}
}
