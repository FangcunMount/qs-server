package classification_test

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
)

func TestFactorGraphDetectsCycle(t *testing.T) {
	graph := classification.FactorGraph{
		Factors: map[classification.FactorID]classification.PersonalityFactor{
			"a": {ID: "a", Kind: classification.FactorKindComposite, Children: []classification.FactorID{"b"}},
			"b": {ID: "b", Kind: classification.FactorKindComposite, Children: []classification.FactorID{"a"}},
		},
		Roots: []classification.FactorID{"a"},
	}
	if err := graph.Validate(); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("Validate() = %v, want cycle error", err)
	}
}

func TestFactorGraphScoresLeafAndCompositeSum(t *testing.T) {
	graph := classification.FactorGraph{
		Factors: map[classification.FactorID]classification.PersonalityFactor{
			"EI": {ID: "EI", Code: "EI", Kind: classification.FactorKindLeaf},
			"SN": {ID: "SN", Code: "SN", Kind: classification.FactorKindLeaf},
			"total": {
				ID:          "total",
				Code:        "total",
				Kind:        classification.FactorKindComposite,
				Children:    []classification.FactorID{"EI", "SN"},
				Aggregation: classification.AggregationSum,
			},
		},
		LeafSpecs: map[classification.FactorID]classification.LeafScoringSpec{
			"EI": {
				Constant: 10,
				Contributions: []classification.AnswerContribution{
					{QuestionCode: "q1", Sign: 1},
				},
			},
			"SN": {
				Constant: 5,
				Contributions: []classification.AnswerContribution{
					{QuestionCode: "q2", Sign: 2},
				},
			},
		},
		Roots: []classification.FactorID{"total"},
	}
	sheet := &classification.AnswerSheet{
		Answers: []classification.Answer{
			{QuestionCode: "q1", Score: 3},
			{QuestionCode: "q2", Score: 4},
		},
	}
	vector, err := classification.ScoreGraph(graph, sheet)
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
	graph := classification.FactorGraph{
		Factors: map[classification.FactorID]classification.PersonalityFactor{
			"EI": {ID: "EI", Kind: classification.FactorKindLeaf},
		},
		LeafSpecs: map[classification.FactorID]classification.LeafScoringSpec{
			"EI": {Contributions: []classification.AnswerContribution{{QuestionCode: "q1", Sign: 1}}},
		},
		Roots: []classification.FactorID{"EI"},
	}
	_, err := classification.ScoreGraph(graph, &classification.AnswerSheet{})
	if err == nil {
		t.Fatal("expected missing answer error")
	}
}

func TestSelectPoleCompositionBuildsTypeCode(t *testing.T) {
	vector := classification.ProfileVector{
		Scores: map[classification.FactorID]classification.FactorScore{
			"EI": {FactorID: "EI", Raw: 30},
			"SN": {FactorID: "SN", Raw: 10},
		},
	}
	outcome, err := classification.SelectOutcome(vector, classification.DecisionSpec{
		Kind: classification.DecisionKindPoleComposition,
		Poles: []classification.PoleSpec{
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
	vector := classification.ProfileVector{
		Scores: map[classification.FactorID]classification.FactorScore{
			"D1": {Raw: 6},
			"D2": {Raw: 6},
		},
	}
	outcome, err := classification.SelectOutcome(vector, classification.DecisionSpec{
		Kind:         classification.DecisionKindNearestPattern,
		PatternOrder: []classification.FactorID{"D1", "D2"},
		LevelRule:    classification.LevelRule{LowMax: 3, HighMin: 5},
		Patterns: []classification.PatternCandidate{
			{Code: "HIGH", Pattern: map[classification.FactorID]string{"D1": "H", "D2": "H"}},
			{Code: "MID", Pattern: map[classification.FactorID]string{"D1": "M", "D2": "M"}},
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
	vector := classification.ProfileVector{
		Scores: map[classification.FactorID]classification.FactorScore{
			"O": {Raw: 3.5},
			"C": {Raw: 4.1},
		},
	}
	outcome, err := classification.SelectOutcome(vector, classification.DecisionSpec{Kind: classification.DecisionKindTraitProfile})
	if err != nil {
		t.Fatalf("SelectOutcome: %v", err)
	}
	if outcome.TraitScores["O"] != 3.5 || outcome.TraitScores["C"] != 4.1 {
		t.Fatalf("traits = %#v", outcome.TraitScores)
	}
}

func TestFactorGraphRejectsNonChildWeight(t *testing.T) {
	graph := classification.FactorGraph{
		Factors: map[classification.FactorID]classification.PersonalityFactor{
			"a": {ID: "a", Kind: classification.FactorKindLeaf},
			"b": {ID: "b", Kind: classification.FactorKindLeaf},
			"total": {
				ID:          "total",
				Kind:        classification.FactorKindComposite,
				Children:    []classification.FactorID{"a", "b"},
				Aggregation: classification.AggregationWeightedAvg,
				Weights:     map[classification.FactorID]float64{"a": 1, "c": 1},
			},
		},
		LeafSpecs: map[classification.FactorID]classification.LeafScoringSpec{
			"a": {Contributions: []classification.AnswerContribution{{QuestionCode: "q1", Sign: 1}}},
			"b": {Contributions: []classification.AnswerContribution{{QuestionCode: "q2", Sign: 1}}},
		},
		Roots: []classification.FactorID{"total"},
	}
	if err := graph.Validate(); err == nil || !strings.Contains(err.Error(), "not a child") {
		t.Fatalf("Validate() = %v, want non-child weight error", err)
	}
}

func TestFactorGraphRejectsMissingChildWeight(t *testing.T) {
	graph := classification.FactorGraph{
		Factors: map[classification.FactorID]classification.PersonalityFactor{
			"a": {ID: "a", Kind: classification.FactorKindLeaf},
			"b": {ID: "b", Kind: classification.FactorKindLeaf},
			"total": {
				ID:          "total",
				Kind:        classification.FactorKindComposite,
				Children:    []classification.FactorID{"a", "b"},
				Aggregation: classification.AggregationWeightedAvg,
				Weights:     map[classification.FactorID]float64{"a": 1},
			},
		},
		LeafSpecs: map[classification.FactorID]classification.LeafScoringSpec{
			"a": {Contributions: []classification.AnswerContribution{{QuestionCode: "q1", Sign: 1}}},
			"b": {Contributions: []classification.AnswerContribution{{QuestionCode: "q2", Sign: 1}}},
		},
		Roots: []classification.FactorID{"total"},
	}
	if err := graph.Validate(); err == nil || !strings.Contains(err.Error(), "missing weight") {
		t.Fatalf("Validate() = %v, want missing child weight error", err)
	}
}

func TestFactorGraphScoresWeightedAverageByChildrenOrder(t *testing.T) {
	graph := classification.FactorGraph{
		Factors: map[classification.FactorID]classification.PersonalityFactor{
			"a": {ID: "a", Code: "a", Kind: classification.FactorKindLeaf},
			"b": {ID: "b", Code: "b", Kind: classification.FactorKindLeaf},
			"total": {
				ID:          "total",
				Code:        "total",
				Kind:        classification.FactorKindComposite,
				Children:    []classification.FactorID{"a", "b"},
				Aggregation: classification.AggregationWeightedAvg,
				Weights:     map[classification.FactorID]float64{"a": 1, "b": 3},
			},
		},
		LeafSpecs: map[classification.FactorID]classification.LeafScoringSpec{
			"a": {Contributions: []classification.AnswerContribution{{QuestionCode: "q1", Sign: 1}}},
			"b": {Contributions: []classification.AnswerContribution{{QuestionCode: "q2", Sign: 2}}},
		},
		Roots: []classification.FactorID{"total"},
	}
	sheet := &classification.AnswerSheet{
		Answers: []classification.Answer{
			{QuestionCode: "q1", Score: 2},
			{QuestionCode: "q2", Score: 4},
		},
	}
	vector, err := classification.ScoreGraph(graph, sheet)
	if err != nil {
		t.Fatalf("ScoreGraph: %v", err)
	}
	if vector.Scores["total"].Raw != 6.5 {
		t.Fatalf("total raw = %v, want 6.5", vector.Scores["total"].Raw)
	}
}

func TestSelectDominantFactorReturnsStableTopK(t *testing.T) {
	vector := classification.ProfileVector{Scores: map[classification.FactorID]classification.FactorScore{
		"A": {Raw: 8},
		"B": {Raw: 10},
		"C": {Raw: 10},
	}}
	outcome, err := classification.SelectOutcome(vector, classification.DecisionSpec{
		Kind:        classification.DecisionKindDominantFactor,
		FactorOrder: []classification.FactorID{"A", "B", "C"},
		TopK:        2,
	})
	if err != nil {
		t.Fatalf("SelectOutcome: %v", err)
	}
	if outcome.Code != "B" || len(outcome.RankedFactors) != 2 || outcome.RankedFactors[1].Code != "C" {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
}
