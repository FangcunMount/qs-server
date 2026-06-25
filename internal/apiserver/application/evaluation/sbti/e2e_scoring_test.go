package sbti

import (
	"testing"

	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestE2EScoreWithEmbeddedSBTIModel(t *testing.T) {
	catalog, err := evaluationinput.NewDefaultSBTIModelCatalog()
	if err != nil {
		t.Fatalf("NewDefaultSBTIModelCatalog: %v", err)
	}
	model, err := catalog.GetSBTIModelByRef(t.Context(), port.ModelRef{Code: port.DefaultSBTIModelCode, Version: port.DefaultSBTIModelVersion})
	if err != nil {
		t.Fatalf("GetSBTIModelByRef: %v", err)
	}

	t.Run("normal_outcome", func(t *testing.T) {
		sheet := allThreesAnswerSheet(model)
		got, err := NewScorer().Score(model, sheet)
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
		sheet := alternatingAnswerSheet(&modelCopy)
		got, err := NewScorer().Score(&modelCopy, sheet)
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
		got, err := NewScorer().Score(model, sheet)
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

func allThreesAnswerSheet(model *port.SBTIModelSnapshot) *port.AnswerSheetSnapshot {
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

func alternatingAnswerSheet(model *port.SBTIModelSnapshot) *port.AnswerSheetSnapshot {
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
