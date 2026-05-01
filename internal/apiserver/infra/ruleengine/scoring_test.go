package ruleengine

import (
	"context"
	"testing"

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

	results, err := NewAnswerScorer(nil).ScoreAnswers(context.Background(), []ruleengineport.AnswerScoreTask{
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
