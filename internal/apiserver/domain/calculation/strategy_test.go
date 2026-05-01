package calculation

import (
	"math"
	"testing"
)

type scorableStub struct {
	empty     bool
	single    string
	hasSingle bool
	multiple  []string
	hasMulti  bool
	number    float64
	hasNumber bool
}

func (v scorableStub) IsEmpty() bool { return v.empty }

func (v scorableStub) AsSingleSelection() (string, bool) {
	return v.single, v.hasSingle
}

func (v scorableStub) AsMultipleSelections() ([]string, bool) {
	return v.multiple, v.hasMulti
}

func (v scorableStub) AsNumber() (float64, bool) {
	return v.number, v.hasNumber
}

func TestBuiltInStrategiesCalculateCurrentBehavior(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		strategy StrategyType
		values   []float64
		params   map[string]string
		want     float64
	}{
		{name: "sum", strategy: StrategyTypeSum, values: []float64{1, 2, 3}, want: 6},
		{name: "average", strategy: StrategyTypeAverage, values: []float64{1, 2, 6}, want: 3},
		{name: "weighted sum default weights", strategy: StrategyTypeWeightedSum, values: []float64{1, 2, 3}, want: 6},
		{name: "weighted sum configured weights", strategy: StrategyTypeWeightedSum, values: []float64{10, 20}, params: map[string]string{ParamKeyWeights: "[0.2,0.5]"}, want: 12},
		{name: "max", strategy: StrategyTypeMax, values: []float64{-1, 4, 2}, want: 4},
		{name: "min", strategy: StrategyTypeMin, values: []float64{-1, 4, 2}, want: -1},
		{name: "count", strategy: StrategyTypeCount, values: []float64{8, 9, 10}, want: 3},
		{name: "first", strategy: StrategyTypeFirst, values: []float64{8, 9, 10}, want: 8},
		{name: "last", strategy: StrategyTypeLast, values: []float64{8, 9, 10}, want: 10},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			strategy := GetStrategy(tc.strategy)
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

	_, err := GetStrategy(StrategyTypeWeightedSum).Calculate([]float64{1, 2}, map[string]string{ParamKeyWeights: "[1]"})
	if err == nil {
		t.Fatal("expected error for mismatched weights")
	}
}

func TestOptionScorerSupportsSelectionAndNumericValues(t *testing.T) {
	t.Parallel()

	scorer := NewOptionScorer()
	optionScores := map[string]float64{"A": 1.5, "B": 2, "C": 3}

	if got := scorer.Score(scorableStub{single: "B", hasSingle: true}, optionScores); got != 2 {
		t.Fatalf("single selection score = %v, want 2", got)
	}
	if got := scorer.Score(scorableStub{multiple: []string{"A", "C"}, hasMulti: true}, optionScores); got != 4.5 {
		t.Fatalf("multiple selection score = %v, want 4.5", got)
	}
	if got := scorer.Score(scorableStub{number: 7, hasNumber: true}, optionScores); got != 7 {
		t.Fatalf("numeric score = %v, want 7", got)
	}
	if got := scorer.Score(scorableStub{single: "missing", hasSingle: true}, optionScores); got != 0 {
		t.Fatalf("missing option score = %v, want 0", got)
	}
}

func TestBatchScorerKeepsTaskOrder(t *testing.T) {
	t.Parallel()

	tasks := make([]ScoreTask, 12)
	for i := range tasks {
		tasks[i] = ScoreTask{
			ID:           string(rune('a' + i)),
			Value:        scorableStub{number: float64(i), hasNumber: true},
			OptionScores: map[string]float64{"unused": 1},
		}
	}

	results := NewBatchScorer().ScoreAllConcurrent(tasks, 3)
	if len(results) != len(tasks) {
		t.Fatalf("result len = %d, want %d", len(results), len(tasks))
	}
	for i := range results {
		if results[i].ID != tasks[i].ID || results[i].Score != float64(i) {
			t.Fatalf("result[%d] = %+v, want id %s score %d", i, results[i], tasks[i].ID, i)
		}
	}
}
