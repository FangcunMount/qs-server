package configured_test

import (
	"testing"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/runtime/configured"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestConfiguredEvaluatorMatchesBigFiveTraitProfile(t *testing.T) {
	payload := bigFivePayload()
	sheet := bigFiveSheet()
	evaluator := configured.NewEvaluator()

	got, err := evaluator.Score(payload, canonicalDefinitionFixture(t, payload), sheet)
	if err != nil {
		t.Fatalf("configured Score: %v", err)
	}
	gotGeneric, err := outcometypology.TraitProfileDetailFromPayload(got.Detail)
	if err != nil {
		t.Fatalf("detail parse: %v", err)
	}
	if len(gotGeneric.Traits) != 2 || gotGeneric.Traits[0].RawScore != 6 || gotGeneric.Traits[1].RawScore != 8 {
		t.Fatalf("traits = %#v, want O=6 C=8", gotGeneric.Traits)
	}
}

func bigFivePayload() *modeltypology.Payload {
	return &modeltypology.Payload{
		Code:      "BIGFIVE_V1",
		Version:   "1.0.0",
		Algorithm: modelcatalog.AlgorithmPersonalityTypology,
		Runtime: &modeltypology.RuntimeSpec{
			FactorGraph: modeltypology.FactorGraphSpec{
				Factors: map[string]modeltypology.FactorSpec{
					"O": {ID: "O", Code: "O", Name: "Openness", Kind: modeltypology.FactorSpecKindLeaf, Contributions: []modeltypology.FactorContributionSpec{
						{QuestionCode: "O1", ScoringMode: modeltypology.QuestionScoringModeQuestionScore, Sign: 1, Weight: 1},
						{QuestionCode: "O2", ScoringMode: modeltypology.QuestionScoringModeQuestionScore, Sign: 1, Weight: 1},
					}},
					"C": {ID: "C", Code: "C", Name: "Conscientiousness", Kind: modeltypology.FactorSpecKindLeaf, Contributions: []modeltypology.FactorContributionSpec{
						{QuestionCode: "C1", ScoringMode: modeltypology.QuestionScoringModeQuestionScore, Sign: 1, Weight: 1},
						{QuestionCode: "C2", ScoringMode: modeltypology.QuestionScoringModeQuestionScore, Sign: 1, Weight: 1},
					}},
				},
				Roots: []string{"O", "C"},
				Dimensions: map[string]modeltypology.Dimension{
					"O": {Code: "O", Name: "Openness"},
					"C": {Code: "C", Name: "Conscientiousness"},
				},
			},
			Decision:       modeltypology.PersonalityDecisionSpec{Kind: modelcatalog.DecisionKindTraitProfile},
			OutcomeMapping: modeltypology.OutcomeMappingSpec{DetailKind: modeltypology.OutcomeDetailTraitProfile, DetailAdapterKey: modeltypology.DetailAdapterTraitProfile},
			Report:         modeltypology.ReportSpec{Kind: modeltypology.ReportKindTraitProfile, AdapterKey: modeltypology.ReportAdapterTraitProfile},
		},
	}
}

func bigFiveSheet() *evalinput.AnswerSheet {
	return &evalinput.AnswerSheet{Answers: []evalinput.Answer{
		{QuestionCode: "O1", Score: 4},
		{QuestionCode: "O2", Score: 2},
		{QuestionCode: "C1", Score: 5},
		{QuestionCode: "C2", Score: 3},
	}}
}
