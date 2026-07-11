package characterization_test

import (
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	mongoevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
)

// V1 contract: SBTI scorer resolves HIGH with similarity=1; report projects
// outcome commentary, rarity, and dimension level descriptions.
func TestV1SBTIPipelinePreservesOutcomeSimilarityAndReportFields(t *testing.T) {
	model := sbtiCharacterizationModel()
	detail, err := typologylegacy.ScoreSBTIReference(model, sbtiHighAnswerSheet())
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if detail.TypeCode != "HIGH" {
		t.Fatalf("TypeCode = %s, want HIGH", detail.TypeCode)
	}
	if detail.Similarity != 1 {
		t.Fatalf("Similarity = %.2f, want 1", detail.Similarity)
	}
	if detail.Dimensions[0].Level != "H" || detail.Dimensions[1].Level != "H" {
		t.Fatalf("dimension levels = %#v, want both H", detail.Dimensions)
	}

	a := submittedSBTIAssessment(t)
	score := 100.0
	result := assessment.NewModelEvaluationResult(
		*a.EvaluationModelRef(),
		assessment.ResultSummary{PrimaryLabel: detail.TypeCode, Score: &score},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindPersonality, Payload: detail},
	)

	report := buildLegacyReport(t, mustConfiguredReportBuilder(t), evaloutcome.NewOutcomeFromLegacyResult(a, nil, result))

	if report.TotalScore() != 100 {
		t.Fatalf("TotalScore = %.1f, want 100", report.TotalScore())
	}
	if report.Conclusion() != "HIGH 高能者 - all high（匹配度 100%）" {
		t.Fatalf("Conclusion = %q", report.Conclusion())
	}

	extra := report.ModelExtra()
	if extra == nil || extra.Kind != "sbti" || extra.TypeCode != "HIGH" {
		t.Fatalf("ModelExtra = %#v, want sbti HIGH", extra)
	}
	if extra.MatchPercent != 100 {
		t.Fatalf("MatchPercent = %.2f, want 100", extra.MatchPercent)
	}
	if extra.Rarity == nil || extra.Rarity.OneInX != 20 {
		t.Fatalf("Rarity = %#v, want one_in_x 20", extra.Rarity)
	}

	dims := report.Dimensions()
	if len(dims) != 2 {
		t.Fatalf("len(Dimensions) = %d, want 2", len(dims))
	}
	assertDimensionField(t, dims[0], "行动力", 6, domainreport.RiskLevelNone, "Alpha / 行动力：H 档，原始分 6/6")

	suggestions := report.Suggestions()
	assertSuggestionExists(t, suggestions, domainreport.SuggestionCategoryGeneral, "你是典型高能者")

	mapper := mongoevaluation.NewReportMapper()
	roundTrip := mapper.ToDomain(mapper.ToPO(report, 8003))
	if roundTrip.ModelExtra() == nil || roundTrip.ModelExtra().TypeCode != "HIGH" {
		t.Fatalf("mongo round trip model extra = %#v", roundTrip.ModelExtra())
	}
	if roundTrip.TotalScore() != 100 {
		t.Fatalf("mongo round trip TotalScore = %.1f, want 100", roundTrip.TotalScore())
	}
}
