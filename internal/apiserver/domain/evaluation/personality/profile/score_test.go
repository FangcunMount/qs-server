package profile_test

import (
	"strings"
	"testing"

	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/profile"
)

func TestOptionScoringStrictRejectsUnknownOption(t *testing.T) {
	graph := optionScoringGraph(profile.OptionScoringStrict)
	_, err := profile.ScoreGraph(graph, &evaluationinput.AnswerSheet{
		Answers: []evaluationinput.Answer{
			{QuestionCode: "q1", Value: "X", Score: 2},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid answer") {
		t.Fatalf("ScoreGraph() = %v, want strict option error", err)
	}
}

func TestOptionScoringCompatFallsBackToAnswerScore(t *testing.T) {
	graph := optionScoringGraph(profile.OptionScoringCompat)
	vector, err := profile.ScoreGraph(graph, &evaluationinput.AnswerSheet{
		Answers: []evaluationinput.Answer{
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

func optionScoringGraph(policy profile.OptionScoringPolicy) profile.FactorGraph {
	return profile.FactorGraph{
		Factors: map[profile.FactorID]profile.PersonalityFactor{
			"D1": {ID: "D1", Code: "D1", Kind: profile.FactorKindLeaf},
		},
		LeafSpecs: map[profile.FactorID]profile.LeafScoringSpec{
			"D1": {
				OptionScoring: policy,
				Contributions: []profile.AnswerContribution{
					{
						QuestionCode: "q1",
						OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3},
					},
				},
			},
		},
		Roots: []profile.FactorID{"D1"},
	}
}
