package ruleset

import (
	"testing"

	evaluationinputdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func TestE2EScoreWithEmbeddedSBTIModel(t *testing.T) {
	model, err := LoadDefaultSBTILegacyModel()
	if err != nil {
		t.Fatalf("LoadDefaultSBTILegacyModel: %v", err)
	}

	t.Run("normal_outcome", func(t *testing.T) {
		sheet := sbtiAllThreesAnswerSheet(model)
		got, err := evaluationtypology.ScoreSBTIReference(model, sheet)
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
		got, err := evaluationtypology.ScoreSBTIReference(&modelCopy, sheet)
		if err != nil {
			t.Fatalf("Score: %v", err)
		}
		if got.TypeCode != "HHHH" {
			t.Fatalf("TypeCode = %s, want HHHH", got.TypeCode)
		}
	})

	t.Run("hidden_DRUNK", func(t *testing.T) {
		sheet := &evaluationinputdomain.AnswerSheet{
			Answers: []evaluationinputdomain.Answer{
				{QuestionCode: "drink_gate_q2", Value: "2"},
			},
		}
		got, err := evaluationtypology.ScoreSBTIReference(model, sheet)
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

func sbtiAllThreesAnswerSheet(model *modeltypology.SBTILegacyModel) *evaluationinputdomain.AnswerSheet {
	answers := make([]evaluationinputdomain.Answer, 0, len(model.QuestionMappings))
	for _, mapping := range model.QuestionMappings {
		answers = append(answers, evaluationinputdomain.Answer{
			QuestionCode: mapping.QuestionCode,
			Value:        "3",
			Score:        3,
		})
	}
	return &evaluationinputdomain.AnswerSheet{
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Answers:              answers,
	}
}

func sbtiAlternatingAnswerSheet(model *modeltypology.SBTILegacyModel) *evaluationinputdomain.AnswerSheet {
	answers := make([]evaluationinputdomain.Answer, 0, len(model.QuestionMappings))
	for i, mapping := range model.QuestionMappings {
		value := "1"
		if i%2 == 1 {
			value = "3"
		}
		answers = append(answers, evaluationinputdomain.Answer{
			QuestionCode: mapping.QuestionCode,
			Value:        value,
			Score:        float64((i % 2) + 1),
		})
	}
	return &evaluationinputdomain.AnswerSheet{
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Answers:              answers,
	}
}
