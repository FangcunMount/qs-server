package profile_test

import (
	"strings"
	"testing"

	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/profile"
)

func TestFactorGraphDetectsCycle(t *testing.T) {
	graph := profile.FactorGraph{
		Factors: map[profile.FactorID]profile.PersonalityFactor{
			"a": {ID: "a", Kind: profile.FactorKindComposite, Children: []profile.FactorID{"b"}},
			"b": {ID: "b", Kind: profile.FactorKindComposite, Children: []profile.FactorID{"a"}},
		},
		Roots: []profile.FactorID{"a"},
	}
	if err := graph.Validate(); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("Validate() = %v, want cycle error", err)
	}
}

func TestFactorGraphScoresLeafAndCompositeSum(t *testing.T) {
	graph := profile.FactorGraph{
		Factors: map[profile.FactorID]profile.PersonalityFactor{
			"EI": {ID: "EI", Code: "EI", Kind: profile.FactorKindLeaf},
			"SN": {ID: "SN", Code: "SN", Kind: profile.FactorKindLeaf},
			"total": {
				ID:          "total",
				Code:        "total",
				Kind:        profile.FactorKindComposite,
				Children:    []profile.FactorID{"EI", "SN"},
				Aggregation: profile.AggregationSum,
			},
		},
		LeafSpecs: map[profile.FactorID]profile.LeafScoringSpec{
			"EI": {
				Constant: 10,
				Contributions: []profile.AnswerContribution{
					{QuestionCode: "q1", Sign: 1},
				},
			},
			"SN": {
				Constant: 5,
				Contributions: []profile.AnswerContribution{
					{QuestionCode: "q2", Sign: 2},
				},
			},
		},
		Roots: []profile.FactorID{"total"},
	}
	sheet := &evaluationinput.AnswerSheet{
		Answers: []evaluationinput.Answer{
			{QuestionCode: "q1", Score: 3},
			{QuestionCode: "q2", Score: 4},
		},
	}
	vector, err := profile.ScoreGraph(graph, sheet)
	if err != nil {
		t.Fatalf("ScoreGraph: %v", err)
	}
	if vector.Scores["EI"].Raw != 13 {
		t.Fatalf("EI raw = %v, want 13", vector.Scores["EI"].Raw)
	}
	if vector.Scores["SN"].Raw != 13 {
		t.Fatalf("SN raw = %v, want 13", vector.Scores["SN"].Raw)
	}
	if vector.Scores["total"].Raw != 26 {
		t.Fatalf("total raw = %v, want 26", vector.Scores["total"].Raw)
	}
}

func TestFactorGraphRejectsMissingAnswer(t *testing.T) {
	graph := profile.FactorGraph{
		Factors: map[profile.FactorID]profile.PersonalityFactor{
			"EI": {ID: "EI", Kind: profile.FactorKindLeaf},
		},
		LeafSpecs: map[profile.FactorID]profile.LeafScoringSpec{
			"EI": {Contributions: []profile.AnswerContribution{{QuestionCode: "q1", Sign: 1}}},
		},
		Roots: []profile.FactorID{"EI"},
	}
	_, err := profile.ScoreGraph(graph, &evaluationinput.AnswerSheet{})
	if err == nil {
		t.Fatal("expected missing answer error")
	}
}

func TestSelectPoleCompositionBuildsTypeCode(t *testing.T) {
	vector := profile.ProfileVector{
		Scores: map[profile.FactorID]profile.FactorScore{
			"EI": {FactorID: "EI", Raw: 30},
			"SN": {FactorID: "SN", Raw: 10},
		},
	}
	outcome, err := profile.SelectOutcome(vector, profile.DecisionSpec{
		Kind: profile.DecisionKindPoleComposition,
		Poles: []profile.PoleSpec{
			{FactorID: "EI", LeftPole: "I", RightPole: "E", Threshold: 24},
			{FactorID: "SN", LeftPole: "S", RightPole: "N", Threshold: 24},
		},
	})
	if err != nil {
		t.Fatalf("SelectOutcome: %v", err)
	}
	if outcome.Code != "ES" {
		t.Fatalf("Code = %s, want ES", outcome.Code)
	}
}

func TestSelectNearestPatternChoosesClosestOutcome(t *testing.T) {
	vector := profile.ProfileVector{
		Scores: map[profile.FactorID]profile.FactorScore{
			"D1": {Raw: 6},
			"D2": {Raw: 6},
		},
	}
	outcome, err := profile.SelectOutcome(vector, profile.DecisionSpec{
		Kind:         profile.DecisionKindNearestPattern,
		PatternOrder: []profile.FactorID{"D1", "D2"},
		LevelRule:    profile.LevelRule{LowMax: 3, HighMin: 5},
		Patterns: []profile.PatternCandidate{
			{Code: "HIGH", Pattern: map[profile.FactorID]string{"D1": "H", "D2": "H"}},
			{Code: "MID", Pattern: map[profile.FactorID]string{"D1": "M", "D2": "M"}},
		},
	})
	if err != nil {
		t.Fatalf("SelectOutcome: %v", err)
	}
	if outcome.Code != "HIGH" || outcome.MatchScore != 1 {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestSelectTraitProfileReturnsFactorScores(t *testing.T) {
	vector := profile.ProfileVector{
		Scores: map[profile.FactorID]profile.FactorScore{
			"O": {Raw: 3.5},
			"C": {Raw: 4.1},
		},
	}
	outcome, err := profile.SelectOutcome(vector, profile.DecisionSpec{Kind: profile.DecisionKindTraitProfile})
	if err != nil {
		t.Fatalf("SelectOutcome: %v", err)
	}
	if outcome.TraitScores["O"] != 3.5 || outcome.TraitScores["C"] != 4.1 {
		t.Fatalf("traits = %#v", outcome.TraitScores)
	}
}

func TestFactorGraphRejectsNonChildWeight(t *testing.T) {
	graph := profile.FactorGraph{
		Factors: map[profile.FactorID]profile.PersonalityFactor{
			"a": {ID: "a", Kind: profile.FactorKindLeaf},
			"b": {ID: "b", Kind: profile.FactorKindLeaf},
			"total": {
				ID:          "total",
				Kind:        profile.FactorKindComposite,
				Children:    []profile.FactorID{"a", "b"},
				Aggregation: profile.AggregationWeightedAvg,
				Weights:     map[profile.FactorID]float64{"a": 1, "c": 1},
			},
		},
		LeafSpecs: map[profile.FactorID]profile.LeafScoringSpec{
			"a": {Contributions: []profile.AnswerContribution{{QuestionCode: "q1", Sign: 1}}},
			"b": {Contributions: []profile.AnswerContribution{{QuestionCode: "q2", Sign: 1}}},
		},
		Roots: []profile.FactorID{"total"},
	}
	if err := graph.Validate(); err == nil || !strings.Contains(err.Error(), "not a child") {
		t.Fatalf("Validate() = %v, want non-child weight error", err)
	}
}

func TestFactorGraphRejectsMissingChildWeight(t *testing.T) {
	graph := profile.FactorGraph{
		Factors: map[profile.FactorID]profile.PersonalityFactor{
			"a": {ID: "a", Kind: profile.FactorKindLeaf},
			"b": {ID: "b", Kind: profile.FactorKindLeaf},
			"total": {
				ID:          "total",
				Kind:        profile.FactorKindComposite,
				Children:    []profile.FactorID{"a", "b"},
				Aggregation: profile.AggregationWeightedAvg,
				Weights:     map[profile.FactorID]float64{"a": 1},
			},
		},
		LeafSpecs: map[profile.FactorID]profile.LeafScoringSpec{
			"a": {Contributions: []profile.AnswerContribution{{QuestionCode: "q1", Sign: 1}}},
			"b": {Contributions: []profile.AnswerContribution{{QuestionCode: "q2", Sign: 1}}},
		},
		Roots: []profile.FactorID{"total"},
	}
	if err := graph.Validate(); err == nil || !strings.Contains(err.Error(), "missing weight") {
		t.Fatalf("Validate() = %v, want missing child weight error", err)
	}
}

func TestFactorGraphScoresWeightedAverageByChildrenOrder(t *testing.T) {
	graph := profile.FactorGraph{
		Factors: map[profile.FactorID]profile.PersonalityFactor{
			"a": {ID: "a", Code: "a", Kind: profile.FactorKindLeaf},
			"b": {ID: "b", Code: "b", Kind: profile.FactorKindLeaf},
			"total": {
				ID:          "total",
				Code:        "total",
				Kind:        profile.FactorKindComposite,
				Children:    []profile.FactorID{"a", "b"},
				Aggregation: profile.AggregationWeightedAvg,
				Weights:     map[profile.FactorID]float64{"a": 1, "b": 3},
			},
		},
		LeafSpecs: map[profile.FactorID]profile.LeafScoringSpec{
			"a": {Contributions: []profile.AnswerContribution{{QuestionCode: "q1", Sign: 1}}},
			"b": {Contributions: []profile.AnswerContribution{{QuestionCode: "q2", Sign: 2}}},
		},
		Roots: []profile.FactorID{"total"},
	}
	sheet := &evaluationinput.AnswerSheet{
		Answers: []evaluationinput.Answer{
			{QuestionCode: "q1", Score: 2},
			{QuestionCode: "q2", Score: 4},
		},
	}
	vector, err := profile.ScoreGraph(graph, sheet)
	if err != nil {
		t.Fatalf("ScoreGraph: %v", err)
	}
	if vector.Scores["total"].Raw != 6.5 {
		t.Fatalf("total raw = %v, want 6.5", vector.Scores["total"].Raw)
	}
}
