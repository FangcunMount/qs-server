package configured_test

import (
	"testing"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/configured"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

func TestEvaluatorScoresExplicitRuntimeWithoutAlgorithm(t *testing.T) {
	payload := explicitPoleCompositionPayload()
	payload.Algorithm = ""

	got, err := configured.NewEvaluator().Score(payload, explicitPoleCompositionSheet())
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	detail, err := evaluationtypology.MBTIResultDetailFromPayload(got.Detail)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.TypeCode != "INTJ" {
		t.Fatalf("TypeCode = %s, want INTJ", detail.TypeCode)
	}
}

func explicitPoleCompositionPayload() *modeltypology.Payload {
	return &modeltypology.Payload{
		Code:                 "CUSTOM_POLE_V1",
		Version:              "1.0.0",
		QuestionnaireCode:    "CUSTOM_POLE_V1",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		DimensionOrder:       []string{"EI", "SN", "TF", "JP"},
		Dimensions: map[string]modeltypology.Dimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
			"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
			"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
			"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
		},
		QuestionMappings: []modeltypology.QuestionMapping{
			{QuestionCode: "Q_EI", Dimension: "EI", Sign: -1},
			{QuestionCode: "Q_SN", Dimension: "SN", Sign: 1},
			{QuestionCode: "Q_TF", Dimension: "TF", Sign: -1},
			{QuestionCode: "Q_JP", Dimension: "JP", Sign: -1},
		},
		Outcomes: []modeltypology.Outcome{
			{Code: "INTJ", Name: "建筑师", OneLiner: "独立战略家"},
		},
		Runtime: &modeltypology.RuntimeSpec{
			FactorGraph: modeltypology.FactorGraphSpec{
				DimensionOrder: []string{"EI", "SN", "TF", "JP"},
				Dimensions: map[string]modeltypology.Dimension{
					"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
					"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
					"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
					"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
				},
				QuestionMappings: []modeltypology.QuestionMapping{
					{QuestionCode: "Q_EI", Dimension: "EI", Sign: -1},
					{QuestionCode: "Q_SN", Dimension: "SN", Sign: 1},
					{QuestionCode: "Q_TF", Dimension: "TF", Sign: -1},
					{QuestionCode: "Q_JP", Dimension: "JP", Sign: -1},
				},
			},
			Decision: modeltypology.PersonalityDecisionSpec{
				Kind: "pole_composition",
			},
			OutcomeMapping: modeltypology.OutcomeMappingSpec{
				DetailKind: modeltypology.OutcomeDetailPersonalityType,
			},
			Report: modeltypology.ReportSpec{
				Kind:          modeltypology.ReportKindPersonalityType,
				CategoryLabel: "Custom Pole Model",
			},
		},
	}
}

func explicitPoleCompositionSheet() *evaluationinput.AnswerSheet {
	return &evaluationinput.AnswerSheet{Answers: []evaluationinput.Answer{
		{QuestionCode: "Q_EI", Score: 1},
		{QuestionCode: "Q_SN", Score: 5},
		{QuestionCode: "Q_TF", Score: 1},
		{QuestionCode: "Q_JP", Score: 1},
	}}
}
