package characterization_test

import (
	"context"
	"testing"

	typologyapp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	bigfiveadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/bigfive"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	mongoevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
)

// V1 contract: Big Five scorer resolves trait-profile raw scores; report exposes
// trait dimensions, distribution summary, and source attribution.
func TestV1BigFivePipelinePreservesTraitScoresAndReportFields(t *testing.T) {
	model := bigFiveCharacterizationModel()
	detail, err := bigfiveadapter.Score(model, bigFiveAnswerSheet())
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if len(detail.Traits) != 5 {
		t.Fatalf("len(Traits) = %d, want 5", len(detail.Traits))
	}
	if detail.Traits[0].Code != "O" || detail.Traits[0].RawScore != 6 {
		t.Fatalf("openness = %#v, want raw 6", detail.Traits[0])
	}
	if detail.Traits[1].Code != "C" || detail.Traits[1].RawScore != 8 {
		t.Fatalf("conscientiousness = %#v, want raw 8", detail.Traits[1])
	}

	a := submittedBigFiveAssessment(t)
	result := assessment.NewModelEvaluationResult(
		*a.EvaluationModelRef(),
		assessment.ResultSummary{PrimaryLabel: detail.Traits[0].Code},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindPersonality, Payload: detail},
	)

	report, err := typologyapp.NewBigFiveReportBuilder().Build(context.Background(), evaluationresult.NewOutcomeFromLegacyResult(a, nil, result))
	if err != nil {
		t.Fatalf("Build report: %v", err)
	}

	if report.RiskLevel() != domainreport.RiskLevelNone {
		t.Fatalf("RiskLevel = %s, want none", report.RiskLevel())
	}
	wantConclusion := "五大人格特质画像 - Openness 6 / Conscientiousness 8 / Extraversion 6 / Agreeableness 8 / Neuroticism 4"
	if report.Conclusion() != wantConclusion {
		t.Fatalf("Conclusion = %q", report.Conclusion())
	}

	extra := report.ModelExtra()
	if extra == nil || extra.Kind != "bigfive" || extra.TypeName != "五大人格特质" {
		t.Fatalf("ModelExtra = %#v, want bigfive trait profile", extra)
	}
	if extra.Commentary != "Openness 6 / Conscientiousness 8 / Extraversion 6 / Agreeableness 8 / Neuroticism 4" {
		t.Fatalf("Commentary = %q", extra.Commentary)
	}

	dims := report.Dimensions()
	if len(dims) != 5 {
		t.Fatalf("len(Dimensions) = %d, want 5", len(dims))
	}
	if dims[0].Kind() != domainreport.DimensionKindTrait {
		t.Fatalf("dimension kind = %s, want trait", dims[0].Kind())
	}
	assertDimensionField(t, dims[0], "Openness", 6, domainreport.RiskLevelNone, "Openness：原始分 6")

	suggestions := report.Suggestions()
	assertSuggestionExists(t, suggestions, domainreport.SuggestionCategoryGeneral, "特质分布：Openness 6 / Conscientiousness 8 / Extraversion 6 / Agreeableness 8 / Neuroticism 4")
	assertSuggestionExists(t, suggestions, domainreport.SuggestionCategoryGeneral, "来源与授权：IPIP；License: CC0；非商业使用: false。")

	mapper := mongoevaluation.NewReportMapper()
	roundTrip := mapper.ToDomain(mapper.ToPO(report, 8004))
	if roundTrip.ModelExtra() == nil || roundTrip.ModelExtra().Kind != "bigfive" {
		t.Fatalf("mongo round trip model extra = %#v", roundTrip.ModelExtra())
	}
	if len(roundTrip.Dimensions()) != 5 {
		t.Fatalf("mongo round trip dimensions = %d, want 5", len(roundTrip.Dimensions()))
	}
	if roundTrip.Dimensions()[0].Kind() != domainreport.DimensionKindTrait {
		t.Fatalf("mongo round trip dimension kind = %s, want trait", roundTrip.Dimensions()[0].Kind())
	}
}
