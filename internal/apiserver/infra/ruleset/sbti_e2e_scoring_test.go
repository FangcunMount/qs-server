package ruleset

import (
	"testing"

	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

func TestE2EScoreWithEmbeddedSBTIModel(t *testing.T) {
	model, err := LoadDefaultSBTILegacyModel()
	if err != nil {
		t.Fatalf("LoadDefaultSBTILegacyModel: %v", err)
	}

	t.Run("normal_outcome", func(t *testing.T) {
		sheet := sbtiAllThreesAnswerSheet(model)
		got, err := typologylegacy.ScoreSBTIReference(model, sheet)
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
		got, err := typologylegacy.ScoreSBTIReference(&modelCopy, sheet)
		if err != nil {
			t.Fatalf("Score: %v", err)
		}
		if got.TypeCode != "HHHH" {
			t.Fatalf("TypeCode = %s, want HHHH", got.TypeCode)
		}
	})

	t.Run("hidden_DRUNK", func(t *testing.T) {
		sheet := &evalinput.AnswerSheet{
			Answers: []evalinput.Answer{
				{QuestionCode: "drink_gate_q2", Value: "2"},
			},
		}
		got, err := typologylegacy.ScoreSBTIReference(model, sheet)
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

func sbtiAllThreesAnswerSheet(model *modeltypology.SBTILegacyModel) *evalinput.AnswerSheet {
	answers := make([]evalinput.Answer, 0, len(model.QuestionMappings))
	for _, mapping := range model.QuestionMappings {
		answers = append(answers, evalinput.Answer{
			QuestionCode: mapping.QuestionCode,
			Value:        "3",
			Score:        3,
		})
	}
	return &evalinput.AnswerSheet{
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Answers:              answers,
	}
}

func sbtiAlternatingAnswerSheet(model *modeltypology.SBTILegacyModel) *evalinput.AnswerSheet {
	answers := make([]evalinput.Answer, 0, len(model.QuestionMappings))
	for i, mapping := range model.QuestionMappings {
		value := "1"
		if i%2 == 1 {
			value = "3"
		}
		answers = append(answers, evalinput.Answer{
			QuestionCode: mapping.QuestionCode,
			Value:        value,
			Score:        float64((i % 2) + 1),
		})
	}
	return &evalinput.AnswerSheet{
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Answers:              answers,
	}
}
