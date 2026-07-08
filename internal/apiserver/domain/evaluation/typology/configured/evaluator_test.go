package configured_test

import (
	"testing"

	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/configured"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func TestConfiguredEvaluatorMatchesBigFiveTraitProfile(t *testing.T) {
	payload := bigFivePayload()
	sheet := bigFiveSheet()
	evaluator := configured.NewEvaluator()

	got, err := evaluator.Score(payload, sheet)
	if err != nil {
		t.Fatalf("configured Score: %v", err)
	}
	gotGeneric, err := evaluationtypology.TraitProfileDetailFromPayload(got.Detail)
	if err != nil {
		t.Fatalf("detail parse: %v", err)
	}
	if len(gotGeneric.Traits) != 2 || gotGeneric.Traits[0].RawScore != 6 || gotGeneric.Traits[1].RawScore != 8 {
		t.Fatalf("traits = %#v, want O=6 C=8", gotGeneric.Traits)
	}
}

func bigFivePayload() *modeltypology.Payload {
	return &modeltypology.Payload{
		Code:           "BIGFIVE_V1",
		Version:        "1.0.0",
		Algorithm:      modelcatalog.AlgorithmBigFive,
		DimensionOrder: []string{"O", "C"},
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
		MatchingSpec: modeltypology.MatchingSpec{Kind: modelcatalog.DecisionKindTraitProfile},
	}
}

func bigFiveSheet() *evaluationinput.AnswerSheet {
	return &evaluationinput.AnswerSheet{Answers: []evaluationinput.Answer{
		{QuestionCode: "O1", Score: 4},
		{QuestionCode: "O2", Score: 2},
		{QuestionCode: "C1", Score: 5},
		{QuestionCode: "C2", Score: 3},
	}}
}
