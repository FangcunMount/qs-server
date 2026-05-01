package ruleengine

import (
	"context"
	"math"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	ruleengineport "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

type scorableStub struct {
	selected string
}

func (s scorableStub) IsEmpty() bool { return false }
func (s scorableStub) AsSingleSelection() (string, bool) {
	return s.selected, true
}
func (s scorableStub) AsMultipleSelections() ([]string, bool) { return nil, false }
func (s scorableStub) AsNumber() (float64, bool)              { return 0, false }

func TestAnswerScorerMapsScoreResultsToPortDTO(t *testing.T) {
	t.Parallel()

	results, err := NewAnswerScorer().ScoreAnswers(context.Background(), []ruleengineport.AnswerScoreTask{
		{
			ID:           "q1",
			Value:        scorableStub{selected: "A"},
			OptionScores: map[string]float64{"A": 2, "B": 1},
		},
	})
	if err != nil {
		t.Fatalf("ScoreAnswers returned error: %v", err)
	}
	if len(results) != 1 || results[0].ID != "q1" || results[0].Score != 2 || results[0].MaxScore != 2 {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestScoringStrategiesCalculateCurrentBehavior(t *testing.T) {
	t.Parallel()

	strategies := newDefaultScoringStrategies()
	cases := []struct {
		name     string
		strategy calculation.StrategyType
		values   []float64
		params   map[string]string
		want     float64
	}{
		{name: "sum", strategy: calculation.StrategyTypeSum, values: []float64{1, 2, 3}, want: 6},
		{name: "average", strategy: calculation.StrategyTypeAverage, values: []float64{1, 2, 6}, want: 3},
		{name: "weighted sum default weights", strategy: calculation.StrategyTypeWeightedSum, values: []float64{1, 2, 3}, want: 6},
		{name: "weighted sum configured weights", strategy: calculation.StrategyTypeWeightedSum, values: []float64{10, 20}, params: map[string]string{calculation.ParamKeyWeights: "[0.2,0.5]"}, want: 12},
		{name: "max", strategy: calculation.StrategyTypeMax, values: []float64{-1, 4, 2}, want: 4},
		{name: "min", strategy: calculation.StrategyTypeMin, values: []float64{-1, 4, 2}, want: -1},
		{name: "count", strategy: calculation.StrategyTypeCount, values: []float64{8, 9, 10}, want: 3},
		{name: "first", strategy: calculation.StrategyTypeFirst, values: []float64{8, 9, 10}, want: 8},
		{name: "last", strategy: calculation.StrategyTypeLast, values: []float64{8, 9, 10}, want: 10},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			strategy := strategies.Get(tc.strategy)
			if strategy == nil {
				t.Fatalf("strategy %s not registered", tc.strategy)
			}
			got, err := strategy.Calculate(tc.values, tc.params)
			if err != nil {
				t.Fatalf("Calculate returned error: %v", err)
			}
			if math.Abs(got-tc.want) > 0.0001 {
				t.Fatalf("Calculate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWeightedSumRejectsInvalidWeights(t *testing.T) {
	t.Parallel()

	_, err := newDefaultScoringStrategies().Get(calculation.StrategyTypeWeightedSum).Calculate(
		[]float64{1, 2},
		map[string]string{calculation.ParamKeyWeights: "[1]"},
	)
	if err == nil {
		t.Fatal("expected error for mismatched weights")
	}
}

func TestOptionScorerSupportsSelectionAndNumericValues(t *testing.T) {
	t.Parallel()

	scorer := &optionScorer{}
	optionScores := map[string]float64{"A": 1.5, "B": 2, "C": 3}

	if got := scorer.score(scorableStub{selected: "B"}, optionScores); got != 2 {
		t.Fatalf("single selection score = %v, want 2", got)
	}
	if got := scorer.score(multiScorableStub{multiple: []string{"A", "C"}}, optionScores); got != 4.5 {
		t.Fatalf("multiple selection score = %v, want 4.5", got)
	}
	if got := scorer.score(numberScorableStub{number: 7}, optionScores); got != 7 {
		t.Fatalf("numeric score = %v, want 7", got)
	}
	if got := scorer.score(scorableStub{selected: "missing"}, optionScores); got != 0 {
		t.Fatalf("missing option score = %v, want 0", got)
	}
}

func TestScaleFactorScorerScoresConfiguredStrategies(t *testing.T) {
	t.Parallel()

	scorer := NewScaleFactorScorer()
	cases := []struct {
		name     string
		strategy string
		values   []float64
		want     float64
	}{
		{name: "sum", strategy: "sum", values: []float64{1, 2, 3}, want: 6},
		{name: "avg", strategy: "avg", values: []float64{2, 4}, want: 3},
		{name: "cnt", strategy: "cnt", values: []float64{1, 1, 1}, want: 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scorer.ScoreFactor(context.Background(), "factor", tc.values, tc.strategy, nil)
			if err != nil {
				t.Fatalf("ScoreFactor() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("ScoreFactor() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestScaleFactorScorerRejectsUnknownStrategy(t *testing.T) {
	t.Parallel()

	if _, err := NewScaleFactorScorer().ScoreFactor(context.Background(), "factor", []float64{1}, "unknown", nil); err == nil {
		t.Fatal("ScoreFactor() error = nil, want unknown strategy error")
	}
}

type multiScorableStub struct {
	multiple []string
}

func (s multiScorableStub) IsEmpty() bool                          { return false }
func (s multiScorableStub) AsSingleSelection() (string, bool)      { return "", false }
func (s multiScorableStub) AsMultipleSelections() ([]string, bool) { return s.multiple, true }
func (s multiScorableStub) AsNumber() (float64, bool)              { return 0, false }

type numberScorableStub struct {
	number float64
}

func (s numberScorableStub) IsEmpty() bool                          { return false }
func (s numberScorableStub) AsSingleSelection() (string, bool)      { return "", false }
func (s numberScorableStub) AsMultipleSelections() ([]string, bool) { return nil, false }
func (s numberScorableStub) AsNumber() (float64, bool)              { return s.number, true }
