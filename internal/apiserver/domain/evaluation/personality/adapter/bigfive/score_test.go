package bigfive_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	bigfiveadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/bigfive"
)

func TestScoreProducesTraitProfile(t *testing.T) {
	payload := bigFiveFixturePayload()
	sheet := &evaluationinput.AnswerSheet{
		Answers: []evaluationinput.Answer{
			{QuestionCode: "O1", Score: 4},
			{QuestionCode: "O2", Score: 2},
			{QuestionCode: "C1", Score: 5},
			{QuestionCode: "C2", Score: 3},
		},
	}
	got, err := bigfiveadapter.Score(payload, sheet)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if len(got.Traits) != 2 {
		t.Fatalf("traits = %d, want 2", len(got.Traits))
	}
	if got.Traits[0].Code != "O" || got.Traits[0].RawScore != 6 {
		t.Fatalf("openness = %#v, want raw 6", got.Traits[0])
	}
	if got.Traits[1].Code != "C" || got.Traits[1].RawScore != 8 {
		t.Fatalf("conscientiousness = %#v, want raw 8", got.Traits[1])
	}
}

func TestBuildFromPayloadRejectsNonTraitMatchingKind(t *testing.T) {
	payload := bigFiveFixturePayload()
	payload.MatchingSpec.Kind = assessmentmodel.DecisionKindPoleComposition
	_, _, err := bigfiveadapter.BuildFromPayload(payload)
	if err == nil {
		t.Fatal("BuildFromPayload error = nil, want trait_profile validation error")
	}
}

func bigFiveFixturePayload() *modeltypology.Payload {
	return &modeltypology.Payload{
		Code:                 "BIGFIVE_V1",
		Version:              "1.0.0",
		Title:                "Big Five",
		QuestionnaireCode:    "BIGFIVE_V1",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		Algorithm:            assessmentmodel.AlgorithmBigFive,
		DimensionOrder:       []string{"O", "C"},
		Dimensions: map[string]modeltypology.Dimension{
			"O": {Code: "O", Name: "Openness"},
			"C": {Code: "C", Name: "Conscientiousness"},
		},
		QuestionMappings: []modeltypology.QuestionMapping{
			{QuestionCode: "O1", Dimension: "O", Sign: 1},
			{QuestionCode: "O2", Dimension: "O", Sign: 1},
			{QuestionCode: "C1", Dimension: "C", Sign: 1},
			{QuestionCode: "C2", Dimension: "C", Sign: 1},
		},
		MatchingSpec: modeltypology.MatchingSpec{
			Kind: assessmentmodel.DecisionKindTraitProfile,
		},
	}
}
