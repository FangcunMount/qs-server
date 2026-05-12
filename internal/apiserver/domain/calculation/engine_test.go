package calculation

import (
	"context"
	"testing"
)

func TestDefaultStrategyRegistryScores(t *testing.T) {
	engine := NewEngine(DefaultStrategyRegistry{})
	testCases := []struct {
		name     string
		strategy string
		values   []float64
		want     float64
	}{
		{name: "sum", strategy: "sum", values: []float64{1, 2, 3}, want: 6},
		{name: "avg", strategy: "avg", values: []float64{2, 4, 6}, want: 4},
		{name: "cnt", strategy: "cnt", values: []float64{9, 8}, want: 2},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := engine.ScoreDimension(context.Background(), Dimension{
				Code:            "f1",
				ScoringStrategy: tc.strategy,
			}, tc.values)
			if err != nil {
				t.Fatalf("ScoreDimension returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ScoreDimension = %.1f, want %.1f", got, tc.want)
			}
		})
	}
}
