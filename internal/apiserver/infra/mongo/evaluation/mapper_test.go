package evaluation

import (
	"testing"
	"time"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

func TestReportMapperRoundTripPreservesInterpretReportFields(t *testing.T) {
	mapper := NewReportMapper()
	createdAt := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	maxScore := 100.0
	original := domainReport.ReconstructInterpretReport(
		domainReport.ID(42),
		"SCL-90",
		"SCL-90",
		72.5,
		domainReport.RiskLevelMedium,
		"medium risk",
		[]domainReport.DimensionInterpret{
			domainReport.NewDimensionInterpret(
				domainReport.NewFactorCode("total"),
				"总分",
				72.5,
				&maxScore,
				domainReport.RiskLevelMedium,
				"dim",
				"watch",
			),
		},
		[]domainReport.Suggestion{
			{Category: domainReport.SuggestionCategoryGeneral, Content: "follow-up"},
		},
		&domainReport.ModelExtra{
			Kind:     "mbti",
			TypeCode: "MBTI_OEJTS",
			TypeName: "MBTI",
		},
		createdAt,
		&updatedAt,
	)

	po := mapper.ToPO(original, 9)
	if po == nil {
		t.Fatal("ToPO returned nil")
	}
	got := mapper.ToDomain(po)
	if got == nil {
		t.Fatal("ToDomain returned nil")
	}
	if got.ID() != original.ID() ||
		got.ScaleName() != original.ScaleName() ||
		got.ScaleCode() != original.ScaleCode() ||
		got.TotalScore() != original.TotalScore() ||
		got.RiskLevel() != original.RiskLevel() ||
		got.Conclusion() != original.Conclusion() {
		t.Fatalf("round trip summary mismatch: got=%#v want=%#v", got, original)
	}
	if len(got.Dimensions()) != 1 || got.Dimensions()[0].FactorCode().String() != "total" {
		t.Fatalf("dimensions = %#v", got.Dimensions())
	}
	if len(got.Suggestions()) != 1 || got.Suggestions()[0].Content != "follow-up" {
		t.Fatalf("suggestions = %#v", got.Suggestions())
	}
	if got.ModelExtra() == nil || got.ModelExtra().TypeCode != "MBTI_OEJTS" {
		t.Fatalf("model extra = %#v", got.ModelExtra())
	}
}
