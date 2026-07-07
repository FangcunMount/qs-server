package trait_test

import (
	"strings"
	"testing"

	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/trait"
)

func TestFactorGraphDetectsCycle(t *testing.T) {
	graph := trait.FactorGraph{
		Factors: map[trait.FactorID]trait.PersonalityFactor{
			"a": {ID: "a", Kind: trait.FactorKindComposite, Children: []trait.FactorID{"b"}},
			"b": {ID: "b", Kind: trait.FactorKindComposite, Children: []trait.FactorID{"a"}},
		},
		Roots: []trait.FactorID{"a"},
	}
	if err := graph.Validate(); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("Validate() = %v, want cycle error", err)
	}
}

func TestFactorGraphScoresLeafAndCompositeSum(t *testing.T) {
	graph := trait.FactorGraph{
		Factors: map[trait.FactorID]trait.PersonalityFactor{
			"EI": {ID: "EI", Code: "EI", Kind: trait.FactorKindLeaf},
			"SN": {ID: "SN", Code: "SN", Kind: trait.FactorKindLeaf},
			"total": {
				ID:          "total",
				Code:        "total",
				Kind:        trait.FactorKindComposite,
				Children:    []trait.FactorID{"EI", "SN"},
				Aggregation: trait.AggregationSum,
			},
		},
		LeafSpecs: map[trait.FactorID]trait.LeafScoringSpec{
			"EI": {
				Constant: 10,
				Contributions: []trait.AnswerContribution{
					{QuestionCode: "q1", Sign: 1},
				},
			},
			"SN": {
				Constant: 5,
				Contributions: []trait.AnswerContribution{
					{QuestionCode: "q2", Sign: 2},
				},
			},
		},
		Roots: []trait.FactorID{"total"},
	}
	sheet := &evaluationinput.AnswerSheet{
		Answers: []evaluationinput.Answer{
			{QuestionCode: "q1", Score: 3},
			{QuestionCode: "q2", Score: 4},
		},
	}
	vector, err := trait.ScoreGraph(graph, sheet)
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
	graph := trait.FactorGraph{
		Factors: map[trait.FactorID]trait.PersonalityFactor{
			"EI": {ID: "EI", Kind: trait.FactorKindLeaf},
		},
		LeafSpecs: map[trait.FactorID]trait.LeafScoringSpec{
			"EI": {Contributions: []trait.AnswerContribution{{QuestionCode: "q1", Sign: 1}}},
		},
		Roots: []trait.FactorID{"EI"},
	}
	_, err := trait.ScoreGraph(graph, &evaluationinput.AnswerSheet{})
	if err == nil {
		t.Fatal("expected missing answer error")
	}
}

func TestSelectPoleCompositionBuildsTypeCode(t *testing.T) {
	vector := trait.ProfileVector{
		Scores: map[trait.FactorID]trait.FactorScore{
			"EI": {FactorID: "EI", Raw: 30},
			"SN": {FactorID: "SN", Raw: 10},
		},
	}
	outcome, err := trait.SelectOutcome(vector, trait.DecisionSpec{
		Kind: trait.DecisionKindPoleComposition,
		Poles: []trait.PoleSpec{
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
	vector := trait.ProfileVector{
		Scores: map[trait.FactorID]trait.FactorScore{
			"D1": {Raw: 6},
			"D2": {Raw: 6},
		},
	}
	outcome, err := trait.SelectOutcome(vector, trait.DecisionSpec{
		Kind:         trait.DecisionKindNearestPattern,
		PatternOrder: []trait.FactorID{"D1", "D2"},
		LevelRule:    trait.LevelRule{LowMax: 3, HighMin: 5},
		Patterns: []trait.PatternCandidate{
			{Code: "HIGH", Pattern: map[trait.FactorID]string{"D1": "H", "D2": "H"}},
			{Code: "MID", Pattern: map[trait.FactorID]string{"D1": "M", "D2": "M"}},
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
	vector := trait.ProfileVector{
		Scores: map[trait.FactorID]trait.FactorScore{
			"O": {Raw: 3.5},
			"C": {Raw: 4.1},
		},
	}
	outcome, err := trait.SelectOutcome(vector, trait.DecisionSpec{Kind: trait.DecisionKindTraitProfile})
	if err != nil {
		t.Fatalf("SelectOutcome: %v", err)
	}
	if outcome.TraitScores["O"] != 3.5 || outcome.TraitScores["C"] != 4.1 {
		t.Fatalf("traits = %#v", outcome.TraitScores)
	}
}

func TestFactorGraphRejectsNonChildWeight(t *testing.T) {
	graph := trait.FactorGraph{
		Factors: map[trait.FactorID]trait.PersonalityFactor{
			"a": {ID: "a", Kind: trait.FactorKindLeaf},
			"b": {ID: "b", Kind: trait.FactorKindLeaf},
			"total": {
				ID:          "total",
				Kind:        trait.FactorKindComposite,
				Children:    []trait.FactorID{"a", "b"},
				Aggregation: trait.AggregationWeightedAvg,
				Weights:     map[trait.FactorID]float64{"a": 1, "c": 1},
			},
		},
		LeafSpecs: map[trait.FactorID]trait.LeafScoringSpec{
			"a": {Contributions: []trait.AnswerContribution{{QuestionCode: "q1", Sign: 1}}},
			"b": {Contributions: []trait.AnswerContribution{{QuestionCode: "q2", Sign: 1}}},
		},
		Roots: []trait.FactorID{"total"},
	}
	if err := graph.Validate(); err == nil || !strings.Contains(err.Error(), "not a child") {
		t.Fatalf("Validate() = %v, want non-child weight error", err)
	}
}

func TestFactorGraphRejectsMissingChildWeight(t *testing.T) {
	graph := trait.FactorGraph{
		Factors: map[trait.FactorID]trait.PersonalityFactor{
			"a": {ID: "a", Kind: trait.FactorKindLeaf},
			"b": {ID: "b", Kind: trait.FactorKindLeaf},
			"total": {
				ID:          "total",
				Kind:        trait.FactorKindComposite,
				Children:    []trait.FactorID{"a", "b"},
				Aggregation: trait.AggregationWeightedAvg,
				Weights:     map[trait.FactorID]float64{"a": 1},
			},
		},
		LeafSpecs: map[trait.FactorID]trait.LeafScoringSpec{
			"a": {Contributions: []trait.AnswerContribution{{QuestionCode: "q1", Sign: 1}}},
			"b": {Contributions: []trait.AnswerContribution{{QuestionCode: "q2", Sign: 1}}},
		},
		Roots: []trait.FactorID{"total"},
	}
	if err := graph.Validate(); err == nil || !strings.Contains(err.Error(), "missing weight") {
		t.Fatalf("Validate() = %v, want missing child weight error", err)
	}
}

func TestFactorGraphScoresWeightedAverageByChildrenOrder(t *testing.T) {
	graph := trait.FactorGraph{
		Factors: map[trait.FactorID]trait.PersonalityFactor{
			"a": {ID: "a", Code: "a", Kind: trait.FactorKindLeaf},
			"b": {ID: "b", Code: "b", Kind: trait.FactorKindLeaf},
			"total": {
				ID:          "total",
				Code:        "total",
				Kind:        trait.FactorKindComposite,
				Children:    []trait.FactorID{"a", "b"},
				Aggregation: trait.AggregationWeightedAvg,
				Weights:     map[trait.FactorID]float64{"a": 1, "b": 3},
			},
		},
		LeafSpecs: map[trait.FactorID]trait.LeafScoringSpec{
			"a": {Contributions: []trait.AnswerContribution{{QuestionCode: "q1", Sign: 1}}},
			"b": {Contributions: []trait.AnswerContribution{{QuestionCode: "q2", Sign: 2}}},
		},
		Roots: []trait.FactorID{"total"},
	}
	sheet := &evaluationinput.AnswerSheet{
		Answers: []evaluationinput.Answer{
			{QuestionCode: "q1", Score: 2},
			{QuestionCode: "q2", Score: 4},
		},
	}
	vector, err := trait.ScoreGraph(graph, sheet)
	if err != nil {
		t.Fatalf("ScoreGraph: %v", err)
	}
	if vector.Scores["total"].Raw != 6.5 {
		t.Fatalf("total raw = %v, want 6.5", vector.Scores["total"].Raw)
	}
}
