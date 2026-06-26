package ruleset

import (
	"testing"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinputdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

func TestE2EScoreWithEmbeddedMBTIModel(t *testing.T) {
	model, err := LoadDefaultMBTILegacyModel()
	if err != nil {
		t.Fatalf("LoadDefaultMBTILegacyModel: %v", err)
	}

	t.Run("all_neutral", func(t *testing.T) {
		sheet := mbtiLikertAnswerSheet(model, "3")
		got, err := evaluationtypology.ScoreMBTI(model, sheet)
		if err != nil {
			t.Fatalf("Score: %v", err)
		}
		if got.TypeCode != "ISFJ" {
			t.Fatalf("TypeCode = %s, want ISFJ (all dimensions tie at threshold)", got.TypeCode)
		}
		if got.MatchPercent != 0 {
			t.Fatalf("MatchPercent = %.2f, want 0 for neutral tie", got.MatchPercent)
		}
		for _, dim := range got.Dimensions {
			if dim.RawScore != 24 {
				t.Fatalf("%s raw score = %.0f, want 24", dim.Code, dim.RawScore)
			}
			if dim.Strength != 0 {
				t.Fatalf("%s strength = %.2f, want 0", dim.Code, dim.Strength)
			}
		}
		if got.TypeName == "" {
			t.Fatal("expected type name")
		}
		if len(got.Dimensions) != 4 {
			t.Fatalf("dimensions = %d, want 4", len(got.Dimensions))
		}
	})

	t.Run("strong_ESTJ_profile", func(t *testing.T) {
		sheet := mbtiPolePreferenceAnswerSheet(model, map[string]string{
			"EI": "E",
			"SN": "S",
			"TF": "T",
			"JP": "J",
		})
		got, err := evaluationtypology.ScoreMBTI(model, sheet)
		if err != nil {
			t.Fatalf("Score: %v", err)
		}
		if got.TypeCode != "ESTJ" {
			t.Fatalf("TypeCode = %s, want ESTJ", got.TypeCode)
		}
		if got.MatchPercent <= 0 {
			t.Fatalf("MatchPercent = %.2f, want > 0", got.MatchPercent)
		}
	})
}

func mbtiPolePreferenceAnswerSheet(model *modeltypology.MBTILegacyModel, prefs map[string]string) *evaluationinputdomain.AnswerSheet {
	answers := make([]evaluationinputdomain.Answer, 0, len(model.QuestionMappings))
	for _, mapping := range model.QuestionMappings {
		meta := model.Dimensions[mapping.Dimension]
		wantRight := prefs[mapping.Dimension] == meta.RightPole
		value := mbtiLikertValueForSign(mapping.Sign, wantRight)
		score := float64(value[0] - '0')
		answers = append(answers, evaluationinputdomain.Answer{
			QuestionCode: mapping.QuestionCode,
			Value:        value,
			Score:        score,
		})
	}
	return &evaluationinputdomain.AnswerSheet{
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

func mbtiLikertAnswerSheet(model *modeltypology.MBTILegacyModel, value string) *evaluationinputdomain.AnswerSheet {
	answers := make([]evaluationinputdomain.Answer, 0, len(model.QuestionMappings))
	score := float64(value[0] - '0')
	for _, mapping := range model.QuestionMappings {
		answers = append(answers, evaluationinputdomain.Answer{
			QuestionCode: mapping.QuestionCode,
			Value:        value,
			Score:        score,
		})
	}
	return &evaluationinputdomain.AnswerSheet{
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Answers:              answers,
	}
}
