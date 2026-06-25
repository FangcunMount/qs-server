package evaluationinput

import (
	"testing"

	evaluationinputdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/sbti"
	rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/sbti"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestE2EScoreWithEmbeddedSBTIModel(t *testing.T) {
	catalog, err := NewDefaultSBTIModelCatalog()
	if err != nil {
		t.Fatalf("NewDefaultSBTIModelCatalog: %v", err)
	}
	model, err := catalog.GetSBTIModelByRef(t.Context(), port.ModelRef{Code: port.DefaultSBTIModelCode, Version: port.DefaultSBTIModelVersion})
	if err != nil {
		t.Fatalf("GetSBTIModelByRef: %v", err)
	}

	t.Run("normal_outcome", func(t *testing.T) {
		sheet := sbtiAllThreesAnswerSheet(model)
		got, err := evaluationsbti.Score(model, sbtiAnswerSheetFromPort(sheet))
		if err != nil {
			t.Fatalf("Score: %v", err)
		}
		if got.TypeCode == "" {
			t.Fatal("expected a normal outcome type code")
		}
		if got.TypeCode == "HHHH" || got.TypeCode == "DRUNK" {
			t.Fatalf("TypeCode = %s, want a standard outcome", got.TypeCode)
		}
		if got.Similarity < model.FallbackSimilarityThreshold {
			t.Fatalf("Similarity = %.2f below threshold", got.Similarity)
		}
	})

	t.Run("fallback_HHHH", func(t *testing.T) {
		modelCopy := *model
		modelCopy.FallbackSimilarityThreshold = 0.95
		sheet := sbtiAlternatingAnswerSheet(&modelCopy)
		got, err := evaluationsbti.Score(&modelCopy, sbtiAnswerSheetFromPort(sheet))
		if err != nil {
			t.Fatalf("Score: %v", err)
		}
		if got.TypeCode != "HHHH" {
			t.Fatalf("TypeCode = %s, want HHHH", got.TypeCode)
		}
	})

	t.Run("hidden_DRUNK", func(t *testing.T) {
		sheet := &port.AnswerSheetSnapshot{
			Answers: []port.AnswerSnapshot{
				{QuestionCode: "drink_gate_q2", Value: "2"},
			},
		}
		got, err := evaluationsbti.Score(model, sbtiAnswerSheetFromPort(sheet))
		if err != nil {
			t.Fatalf("Score: %v", err)
		}
		if got.TypeCode != "DRUNK" {
			t.Fatalf("TypeCode = %s, want DRUNK", got.TypeCode)
		}
		if got.ImageURL == "" {
			t.Fatal("expected image url for DRUNK outcome")
		}
	})
}

func sbtiAllThreesAnswerSheet(model *rulesetsbti.ModelSnapshot) *port.AnswerSheetSnapshot {
	answers := make([]port.AnswerSnapshot, 0, len(model.QuestionMappings))
	for _, mapping := range model.QuestionMappings {
		answers = append(answers, port.AnswerSnapshot{
			QuestionCode: mapping.QuestionCode,
			Value:        "3",
			Score:        3,
		})
	}
	return &port.AnswerSheetSnapshot{
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Answers:              answers,
	}
}

func sbtiAlternatingAnswerSheet(model *rulesetsbti.ModelSnapshot) *port.AnswerSheetSnapshot {
	answers := make([]port.AnswerSnapshot, 0, len(model.QuestionMappings))
	for i, mapping := range model.QuestionMappings {
		value := "1"
		if i%2 == 1 {
			value = "3"
		}
		answers = append(answers, port.AnswerSnapshot{
			QuestionCode: mapping.QuestionCode,
			Value:        value,
			Score:        float64((i % 2) + 1),
		})
	}
	return &port.AnswerSheetSnapshot{
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Answers:              answers,
	}
}

func sbtiAnswerSheetFromPort(sheet *port.AnswerSheetSnapshot) *evaluationinputdomain.AnswerSheet {
	if sheet == nil {
		return nil
	}
	answers := make([]evaluationinputdomain.Answer, 0, len(sheet.Answers))
	for _, answer := range sheet.Answers {
		answers = append(answers, evaluationinputdomain.Answer{
			QuestionCode: answer.QuestionCode,
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &evaluationinputdomain.AnswerSheet{
		QuestionnaireCode:    sheet.QuestionnaireCode,
		QuestionnaireVersion: sheet.QuestionnaireVersion,
		Answers:              answers,
	}
}
