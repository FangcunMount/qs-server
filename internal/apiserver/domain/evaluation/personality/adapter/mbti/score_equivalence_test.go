package mbti_test

import (
	"testing"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	mbtiadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/mbti"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

func TestScoreMatchesLegacyScorerForUnitModel(t *testing.T) {
	model := mbtiScorerTestModel()
	cases := []struct {
		name  string
		sheet *evaluationinput.AnswerSheet
	}{
		{
			name: "intj_profile",
			sheet: &evaluationinput.AnswerSheet{
				Answers: []evaluationinput.Answer{
					{QuestionCode: "Q_EI", Score: 1},
					{QuestionCode: "Q_SN", Score: 5},
					{QuestionCode: "Q_TF", Score: 1},
					{QuestionCode: "Q_JP", Score: 1},
				},
			},
		},
		{
			name: "estj_profile",
			sheet: mbtiPolePreferenceAnswerSheet(model, map[string]string{
				"EI": "E",
				"SN": "S",
				"TF": "T",
				"JP": "J",
			}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			legacy, err := evaluationtypology.ScoreMBTIReference(model, tc.sheet)
			if err != nil {
				t.Fatalf("ScoreMBTI: %v", err)
			}
			got, err := mbtiadapter.Score(model, tc.sheet)
			if err != nil {
				t.Fatalf("adapter Score: %v", err)
			}
			assertMBTIResultEqual(t, legacy, got)
		})
	}
}

func TestBuildFromLegacyProducesValidGraph(t *testing.T) {
	model := mbtiScorerTestModel()
	graph, spec, err := mbtiadapter.BuildFromLegacy(model)
	if err != nil {
		t.Fatalf("BuildFromLegacy: %v", err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if spec.Kind != "pole_composition" || len(spec.Poles) != 4 {
		t.Fatalf("spec = %#v", spec)
	}
}

func assertMBTIResultEqual(t *testing.T, want, got evaluationtypology.MBTIResultDetail) {
	t.Helper()
	if got.TypeCode != want.TypeCode {
		t.Fatalf("TypeCode = %s, want %s", got.TypeCode, want.TypeCode)
	}
	if got.TypeName != want.TypeName {
		t.Fatalf("TypeName = %s, want %s", got.TypeName, want.TypeName)
	}
	if got.OneLiner != want.OneLiner {
		t.Fatalf("OneLiner = %s, want %s", got.OneLiner, want.OneLiner)
	}
	if got.MatchPercent != want.MatchPercent {
		t.Fatalf("MatchPercent = %.4f, want %.4f", got.MatchPercent, want.MatchPercent)
	}
	if got.ImageURL != want.ImageURL {
		t.Fatalf("ImageURL = %s, want %s", got.ImageURL, want.ImageURL)
	}
	if len(got.Dimensions) != len(want.Dimensions) {
		t.Fatalf("dimensions = %d, want %d", len(got.Dimensions), len(want.Dimensions))
	}
	for i := range want.Dimensions {
		wd := want.Dimensions[i]
		gd := got.Dimensions[i]
		if gd != wd {
			t.Fatalf("dimension[%d] = %#v, want %#v", i, gd, wd)
		}
	}
	if got.Profile.TypeCode != want.Profile.TypeCode {
		t.Fatalf("profile type = %s, want %s", got.Profile.TypeCode, want.Profile.TypeCode)
	}
}

func mbtiScorerTestModel() *modeltypology.MBTILegacyModel {
	return &modeltypology.MBTILegacyModel{
		Code:                 "MBTI_TEST",
		Version:              "1.0.0",
		Title:                "MBTI 测试",
		QuestionnaireCode:    "MBTI_TEST",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		DimensionOrder:       []string{"EI", "SN", "TF", "JP"},
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
			{
				TypeCode:    "INTJ",
				TypeName:    "建筑师",
				OneLiner:    "独立战略家",
				Summary:     "善于长远规划",
				Strengths:   []string{"系统思考"},
				Weaknesses:  []string{"表达克制"},
				Suggestions: []string{"保留沟通空间"},
			},
			{
				TypeCode: "ESTJ",
				TypeName: "总经理",
				OneLiner: "高效组织者",
			},
		},
		Source: modeltypology.MBTILegacySource{
			Attribution:   "OEJTS",
			License:       "CC BY-NC-SA 4.0",
			NonCommercial: true,
		},
	}
}

func mbtiPolePreferenceAnswerSheet(model *modeltypology.MBTILegacyModel, prefs map[string]string) *evaluationinput.AnswerSheet {
	answers := make([]evaluationinput.Answer, 0, len(model.QuestionMappings))
	for _, mapping := range model.QuestionMappings {
		meta := model.Dimensions[mapping.Dimension]
		wantRight := prefs[mapping.Dimension] == meta.RightPole
		value := mbtiLikertValueForSign(mapping.Sign, wantRight)
		score := float64(value[0] - '0')
		answers = append(answers, evaluationinput.Answer{
			QuestionCode: mapping.QuestionCode,
			Value:        value,
			Score:        score,
		})
	}
	return &evaluationinput.AnswerSheet{
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Answers:              answers,
	}
}

func mbtiLikertValueForSign(sign float64, wantRight bool) string {
	if sign > 0 {
		if wantRight {
			return "5"
		}
		return "1"
	}
	if wantRight {
		return "1"
	}
	return "5"
}
