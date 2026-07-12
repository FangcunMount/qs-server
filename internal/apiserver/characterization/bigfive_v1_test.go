package characterization_test

import (
	"testing"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// V1 contract: Big Five scorer resolves trait-profile raw scores; report exposes
// trait dimensions, distribution summary, and source attribution.
func TestV1BigFivePipelinePreservesTraitScoresAndReportFields(t *testing.T) {
	model := bigFiveCharacterizationModel()
	detail, err := scoreBigFiveCharacterization(t, model, bigFiveAnswerSheet())
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
	report := buildPreviewReport(t, mustConfiguredReportBuilder(t), canonicalOutcome(
		t, a, nil,
		domainoutcome.Summary{PrimaryLabel: detail.Traits[0].Code},
		domainoutcome.Detail{Kind: modelcatalog.KindTypology, Payload: detail},
	))

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

}
