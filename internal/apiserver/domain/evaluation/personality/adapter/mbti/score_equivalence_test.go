package mbti_test

import (
	"testing"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	mbtiadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/mbti"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
)

func TestScoreMatchesLegacyScorerForEmbeddedModel(t *testing.T) {
	model, err := ruleset.LoadDefaultMBTILegacyModel()
	if err != nil {
		t.Fatalf("LoadDefaultMBTILegacyModel: %v", err)
	}

	cases := []struct {
		name  string
		sheet *evaluationinput.AnswerSheet
	}{
		{
			name:  "all_neutral",
			sheet: mbtiLikertAnswerSheet(model, "3"),
		},
		{
			name: "strong_ESTJ_profile",
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
			legacy, err := evaluationtypology.ScoreMBTI(model, tc.sheet)
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
	model, err := ruleset.LoadDefaultMBTILegacyModel()
	if err != nil {
		t.Fatalf("LoadDefaultMBTILegacyModel: %v", err)
	}
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

func mbtiLikertAnswerSheet(model *modeltypology.MBTILegacyModel, value string) *evaluationinput.AnswerSheet {
	answers := make([]evaluationinput.Answer, 0, len(model.QuestionMappings))
	score := float64(value[0] - '0')
	for _, mapping := range model.QuestionMappings {
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
