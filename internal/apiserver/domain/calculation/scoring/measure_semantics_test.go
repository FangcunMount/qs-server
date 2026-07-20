package scoring

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestEvaluatorAppliesQuestionContributionSignAndWeight(t *testing.T) {
	t.Parallel()
	input := Input{
		Model: Model{Factors: []Factor{{
			Code:            "dim",
			ScoringStrategy: string(StrategySum),
			Contributions: []QuestionContribution{
				{Code: "q1", Sign: -1, Weight: 0.5},
				{Code: "q2", Sign: 1, Weight: 2},
			},
		}}},
		AnswerSheet: &AnswerSheet{Answers: []Answer{
			{QuestionCode: meta.NewCode("q1"), Score: 4},
			{QuestionCode: meta.NewCode("q2"), Score: 3},
		}},
	}
	// (-1)*0.5*4 + 1*2*3 = -2 + 6 = 4
	result, err := NewDefaultEvaluator().Score(context.Background(), input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	assertFactorScore(t, result.FactorScores, "dim", 4)
}

func TestEvaluatorScoresCompositeFromChildFactors(t *testing.T) {
	t.Parallel()
	input := Input{
		Model: Model{Factors: []Factor{
			{
				Code:            "total",
				IsTotalScore:    true,
				ScoringStrategy: string(StrategySum),
				ChildCodes:      []string{"dim_a", "dim_b"},
			},
			{
				Code:            "dim_a",
				ScoringStrategy: string(StrategySum),
				QuestionCodes:   []string{"q1"},
			},
			{
				Code:            "dim_b",
				ScoringStrategy: string(StrategySum),
				QuestionCodes:   []string{"q2"},
			},
		}},
		AnswerSheet: &AnswerSheet{Answers: []Answer{
			{QuestionCode: meta.NewCode("q1"), Score: 3},
			{QuestionCode: meta.NewCode("q2"), Score: 5},
		}},
	}
	result, err := NewDefaultEvaluator().Score(context.Background(), input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	assertFactorScore(t, result.FactorScores, "dim_a", 3)
	assertFactorScore(t, result.FactorScores, "dim_b", 5)
	assertFactorScore(t, result.FactorScores, "total", 8)
	if result.TotalScore != 8 {
		t.Fatalf("TotalScore = %v, want 8", result.TotalScore)
	}
}

func TestEvaluatorScoresCompositeWeightedSum(t *testing.T) {
	t.Parallel()
	input := Input{
		Model: Model{Factors: []Factor{
			{
				Code:            "index",
				ScoringStrategy: string(StrategyWeightedSum),
				ChildCodes:      []string{"dim_a", "dim_b"},
				ChildWeights:    map[string]float64{"dim_a": 0.5, "dim_b": 2},
			},
			{
				Code:            "dim_a",
				ScoringStrategy: string(StrategySum),
				QuestionCodes:   []string{"q1"},
			},
			{
				Code:            "dim_b",
				ScoringStrategy: string(StrategySum),
				QuestionCodes:   []string{"q2"},
			},
		}},
		AnswerSheet: &AnswerSheet{Answers: []Answer{
			{QuestionCode: meta.NewCode("q1"), Score: 4},
			{QuestionCode: meta.NewCode("q2"), Score: 3},
		}},
	}
	// 4*0.5 + 3*2 = 2 + 6 = 8
	result, err := NewDefaultEvaluator().Score(context.Background(), input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	assertFactorScore(t, result.FactorScores, "index", 8)
}

func TestEvaluatorAppliesOptionOverrideContribution(t *testing.T) {
	t.Parallel()
	input := Input{
		Model: Model{Factors: []Factor{{
			Code:            "dim",
			ScoringStrategy: string(StrategySum),
			Contributions: []QuestionContribution{{
				Code:         "q1",
				ScoringMode:  "option_override",
				OptionScores: map[string]float64{"A": 10, "B": 1},
			}},
		}}},
		AnswerSheet: &AnswerSheet{Answers: []Answer{
			{QuestionCode: meta.NewCode("q1"), Score: 99, Value: "A"},
		}},
	}
	result, err := NewDefaultEvaluator().Score(context.Background(), input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	assertFactorScore(t, result.FactorScores, "dim", 10)
}
