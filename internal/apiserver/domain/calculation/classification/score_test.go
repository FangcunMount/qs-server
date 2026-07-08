package classification_test

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
)

func TestOptionScoringStrictRejectsUnknownOption(t *testing.T) {
	graph := optionScoringGraph(classification.OptionScoringStrict)
	_, err := classification.ScoreGraph(graph, &classification.AnswerSheet{
		Answers: []classification.Answer{
			{QuestionCode: "q1", Value: "X", Score: 2},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid answer") {
		t.Fatalf("ScoreGraph() = %v, want strict option error", err)
	}
}

func TestOptionScoringCompatFallsBackToAnswerScore(t *testing.T) {
	graph := optionScoringGraph(classification.OptionScoringCompat)
	vector, err := classification.ScoreGraph(graph, &classification.AnswerSheet{
		Answers: []classification.Answer{
			{QuestionCode: "q1", Value: "X", Score: 2},
		},
	})
	if err != nil {
		t.Fatalf("ScoreGraph: %v", err)
	}
	if vector.Scores["D1"].Raw != 2 {
		t.Fatalf("D1 raw = %v, want 2", vector.Scores["D1"].Raw)
	}
}

func optionScoringGraph(policy classification.OptionScoringPolicy) classification.FactorGraph {
	return classification.FactorGraph{
		Factors: map[classification.FactorID]classification.PersonalityFactor{
			"D1": {ID: "D1", Code: "D1", Kind: classification.FactorKindLeaf},
		},
		LeafSpecs: map[classification.FactorID]classification.LeafScoringSpec{
			"D1": {
				Contributions: []classification.AnswerContribution{
					{QuestionCode: "q1", OptionScores: map[string]float64{"A": 1}},
				},
				OptionScoring: policy,
			},
		},
		Roots: []classification.FactorID{"D1"},
	}
}
