package configured_test

import (
	"testing"

	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/configured"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestConfiguredEvaluatorMatchesMBTIReference(t *testing.T) {
	model := mbtiModel()
	payload := modeltypology.FromMBTI(model)
	sheet := mbtiSheet()
	evaluator := configured.NewEvaluator()

	got, err := evaluator.Score(payload, sheet)
	if err != nil {
		t.Fatalf("configured Score: %v", err)
	}
	wantLegacy, err := evaluationtypology.ScoreMBTIReference(model, sheet)
	if err != nil {
		t.Fatalf("reference Score: %v", err)
	}
	wantGeneric := evaluationtypology.PersonalityTypeDetailFromMBTI(wantLegacy)
	gotGeneric, err := evaluationtypology.PersonalityTypeDetailFromPayload(got.Detail)
	if err != nil {
		t.Fatalf("detail parse: %v", err)
	}
	if diff := cmp.Diff(wantGeneric, gotGeneric, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("detail mismatch (-want +got):\n%s", diff)
	}
}

func TestConfiguredEvaluatorMatchesSBTIReferenceSpecialPaths(t *testing.T) {
	evaluator := configured.NewEvaluator()

	t.Run("HIGH", func(t *testing.T) {
		payload := modeltypology.FromSBTI(sbtiModel())
		sheet := sbtiHighSheet()
		assertSBTIReferenceEquivalence(t, evaluator, payload, sheet)
	})
	t.Run("DRUNK", func(t *testing.T) {
		payload := modeltypology.FromSBTI(sbtiModel())
		sheet := &evaluationinput.AnswerSheet{Answers: []evaluationinput.Answer{
			{QuestionCode: "drink_gate_q2", Value: "C"},
		}}
		assertSBTIReferenceEquivalence(t, evaluator, payload, sheet)
	})
	t.Run("HHHH", func(t *testing.T) {
		model := sbtiModel()
		model.FallbackSimilarityThreshold = 0.9
		payload := modeltypology.FromSBTI(model)
		sheet := sbtiLowSheet()
		assertSBTIReferenceEquivalence(t, evaluator, payload, sheet)
	})
}

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

func assertSBTIReferenceEquivalence(
	t *testing.T,
	evaluator configured.Evaluator,
	payload *modeltypology.Payload,
	sheet *evaluationinput.AnswerSheet,
) {
	t.Helper()
	model, err := modeltypology.ToSBTI(payload)
	if err != nil {
		t.Fatalf("ToSBTI: %v", err)
	}
	got, err := evaluator.Score(payload, sheet)
	if err != nil {
		t.Fatalf("configured Score: %v", err)
	}
	wantLegacy, err := evaluationtypology.ScoreSBTIReference(model, sheet)
	if err != nil {
		t.Fatalf("reference Score: %v", err)
	}
	wantGeneric := evaluationtypology.PersonalityTypeDetailFromSBTI(wantLegacy)
	gotGeneric, err := evaluationtypology.PersonalityTypeDetailFromPayload(got.Detail)
	if err != nil {
		t.Fatalf("detail parse: %v", err)
	}
	if diff := cmp.Diff(wantGeneric, gotGeneric, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("detail mismatch (-want +got):\n%s", diff)
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

func mbtiModel() *modeltypology.MBTILegacyModel {
	return &modeltypology.MBTILegacyModel{
		Code:           "MBTI_TEST",
		Version:        "1.0.0",
		DimensionOrder: []string{"EI", "SN", "TF", "JP"},
		Dimensions: map[string]modeltypology.MBTILegacyDimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
			"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
			"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
			"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
		},
		QuestionMappings: []modeltypology.MBTILegacyQuestionMapping{
			{QuestionCode: "Q_EI", Dimension: "EI", Sign: -1},
			{QuestionCode: "Q_SN", Dimension: "SN", Sign: 1},
			{QuestionCode: "Q_TF", Dimension: "TF", Sign: -1},
			{QuestionCode: "Q_JP", Dimension: "JP", Sign: -1},
		},
		TypeProfiles: []modeltypology.MBTILegacyTypeProfile{
			{TypeCode: "INTJ", TypeName: "建筑师", OneLiner: "独立战略家"},
		},
	}
}

func mbtiSheet() *evaluationinput.AnswerSheet {
	return &evaluationinput.AnswerSheet{Answers: []evaluationinput.Answer{
		{QuestionCode: "Q_EI", Score: 1},
		{QuestionCode: "Q_SN", Score: 5},
		{QuestionCode: "Q_TF", Score: 1},
		{QuestionCode: "Q_JP", Score: 1},
	}}
}

func sbtiModel() *modeltypology.SBTILegacyModel {
	return &modeltypology.SBTILegacyModel{
		Code:                        "SBTI_FUN",
		Version:                     "1.0.0",
		FallbackSimilarityThreshold: 0.6,
		DimensionOrder:              []string{"D1", "D2"},
		Dimensions: map[string]modeltypology.SBTILegacyDimension{
			"D1": {Code: "D1", Name: "D1", Model: "M1"},
			"D2": {Code: "D2", Name: "D2", Model: "M2"},
		},
		QuestionMappings: []modeltypology.SBTILegacyQuestionMapping{
			{QuestionCode: "Q1", Dimension: "D1", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
			{QuestionCode: "Q2", Dimension: "D1", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
			{QuestionCode: "Q3", Dimension: "D2", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
			{QuestionCode: "Q4", Dimension: "D2", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
		},
		NormalOutcomes: []modeltypology.SBTILegacyOutcome{
			{Code: "HIGH", Name: "高能者", Pattern: "HH", OneLiner: "all high"},
		},
		SpecialOutcomes: []modeltypology.SBTILegacyOutcome{
			{Code: "HHHH", Name: "傻乐者", Trigger: "fallback:best_match<60%", IsSpecial: true},
			{Code: "DRUNK", Name: "酒鬼", Trigger: "hidden:drink_gate_q2_answer=2", IsSpecial: true},
		},
		DrinkTrigger: modeltypology.SBTILegacyDrinkTrigger{
			QuestionCodes: []string{"drink_gate_q2"},
			OptionValues:  []string{"C"},
		},
	}
}

func sbtiHighSheet() *evaluationinput.AnswerSheet {
	return &evaluationinput.AnswerSheet{Answers: []evaluationinput.Answer{
		{QuestionCode: "Q1", Value: "C"},
		{QuestionCode: "Q2", Value: "C"},
		{QuestionCode: "Q3", Value: "C"},
		{QuestionCode: "Q4", Value: "C"},
	}}
}

func sbtiLowSheet() *evaluationinput.AnswerSheet {
	return &evaluationinput.AnswerSheet{Answers: []evaluationinput.Answer{
		{QuestionCode: "Q1", Value: "A"},
		{QuestionCode: "Q2", Value: "A"},
		{QuestionCode: "Q3", Value: "A"},
		{QuestionCode: "Q4", Value: "A"},
	}}
}
