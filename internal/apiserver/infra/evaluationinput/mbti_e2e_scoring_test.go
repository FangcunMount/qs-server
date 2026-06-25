package evaluationinput

import (
	"testing"

	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestE2EScoreWithEmbeddedMBTIModel(t *testing.T) {
	catalog, err := NewDefaultMBTIModelCatalog()
	if err != nil {
		t.Fatalf("NewDefaultMBTIModelCatalog: %v", err)
	}
	model, err := catalog.GetMBTIModelByRef(t.Context(), port.ModelRef{
		Kind:    port.EvaluationModelKindMBTI,
		Code:    port.DefaultMBTIModelCode,
		Version: port.DefaultMBTIModelVersion,
	})
	if err != nil {
		t.Fatalf("GetMBTIModelByRef: %v", err)
	}

	t.Run("all_neutral", func(t *testing.T) {
		sheet := mbtiLikertAnswerSheet(model, "3")
		got, err := evaluationdomain.ScoreMBTI(model, mbtiAnswerSheetFromPort(sheet))
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
		got, err := evaluationdomain.ScoreMBTI(model, mbtiAnswerSheetFromPort(sheet))
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

func mbtiPolePreferenceAnswerSheet(model *port.MBTIModelSnapshot, prefs map[string]string) *port.AnswerSheetSnapshot {
	answers := make([]port.AnswerSnapshot, 0, len(model.QuestionMappings))
	for _, mapping := range model.QuestionMappings {
		meta := model.Dimensions[mapping.Dimension]
		wantRight := prefs[mapping.Dimension] == meta.RightPole
		value := mbtiLikertValueForSign(mapping.Sign, wantRight)
		score := float64(value[0] - '0')
		answers = append(answers, port.AnswerSnapshot{
			QuestionCode: mapping.QuestionCode,
			Value:        value,
			Score:        score,
		})
	}
	return &port.AnswerSheetSnapshot{
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

func mbtiLikertAnswerSheet(model *port.MBTIModelSnapshot, value string) *port.AnswerSheetSnapshot {
	answers := make([]port.AnswerSnapshot, 0, len(model.QuestionMappings))
	score := float64(value[0] - '0')
	for _, mapping := range model.QuestionMappings {
		answers = append(answers, port.AnswerSnapshot{
			QuestionCode: mapping.QuestionCode,
			Value:        value,
			Score:        score,
		})
	}
	return &port.AnswerSheetSnapshot{
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Answers:              answers,
	}
}

func mbtiAnswerSheetFromPort(sheet *port.AnswerSheetSnapshot) *evaluationdomain.AnswerSheet {
	if sheet == nil {
		return nil
	}
	answers := make([]evaluationdomain.Answer, 0, len(sheet.Answers))
	for _, answer := range sheet.Answers {
		answers = append(answers, evaluationdomain.Answer{
			QuestionCode: answer.QuestionCode,
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &evaluationdomain.AnswerSheet{
		QuestionnaireCode:    sheet.QuestionnaireCode,
		QuestionnaireVersion: sheet.QuestionnaireVersion,
		Answers:              answers,
	}
}
