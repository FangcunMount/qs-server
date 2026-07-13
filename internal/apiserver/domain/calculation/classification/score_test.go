package classification_test

import (
	"math"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
)

func TestQuestionScoreContributionUsesArbitraryFiniteAnswerScore(t *testing.T) {
	tests := []struct {
		name   string
		score  float64
		sign   float64
		weight float64
		want   float64
	}{
		{name: "zero", score: 0, sign: 1, weight: 1, want: 0},
		{name: "negative decimal", score: -2.5, sign: -1, weight: 2, want: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := classification.CalculateQuestionContribution(classification.OptionScoringStrict, classification.AnswerContribution{
				QuestionCode: "q1", ScoringMode: classification.QuestionScoringModeQuestionScore, Sign: tt.sign, Weight: tt.weight,
			}, classification.Answer{QuestionCode: "q1", Score: tt.score})
			if err != nil {
				t.Fatalf("CalculateQuestionContribution: %v", err)
			}
			if got != tt.want {
				t.Fatalf("score = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOptionOverrideAppliesSignAndWeight(t *testing.T) {
	got, err := classification.CalculateQuestionContribution(classification.OptionScoringCompat, classification.AnswerContribution{
		QuestionCode: "q1", ScoringMode: classification.QuestionScoringModeOptionOverride, Sign: -1, Weight: 0.5,
		OptionScores: map[string]float64{"A": 4},
	}, classification.Answer{QuestionCode: "q1", Value: "A", Score: 99})
	if err != nil {
		t.Fatalf("CalculateQuestionContribution: %v", err)
	}
	if got != -2 {
		t.Fatalf("score = %v, want -2", got)
	}
}

func TestExplicitContributionRejectsNonFiniteScore(t *testing.T) {
	_, err := classification.CalculateQuestionContribution(classification.OptionScoringStrict, classification.AnswerContribution{
		QuestionCode: "q1", ScoringMode: classification.QuestionScoringModeQuestionScore,
	}, classification.Answer{QuestionCode: "q1", Score: math.Inf(1)})
	if err == nil || !strings.Contains(err.Error(), "finite") {
		t.Fatalf("error = %v, want finite score error", err)
	}
}

func TestLegacyOptionOverrideStillIgnoresSign(t *testing.T) {
	got, err := classification.CalculateQuestionContribution(classification.OptionScoringStrict, classification.AnswerContribution{
		QuestionCode: "q1", Sign: -1, OptionScores: map[string]float64{"A": 4},
	}, classification.Answer{QuestionCode: "q1", Value: "A"})
	if err != nil {
		t.Fatalf("CalculateQuestionContribution: %v", err)
	}
	if got != 4 {
		t.Fatalf("legacy score = %v, want 4", got)
	}
}

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
