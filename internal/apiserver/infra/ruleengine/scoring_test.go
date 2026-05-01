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
