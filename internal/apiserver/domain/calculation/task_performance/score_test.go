package task_performance

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
)

func TestScoreSPMUsesFrozenAnswerKeys(t *testing.T) {
	t.Parallel()
	got := ScoreSPM(
		map[string]string{"A1": "1", "A2": "2", "A3": "1"},
		[]ItemSet{{
			Code: "A",
			Items: []Item{
				{QuestionCode: "A1", CorrectOptionCode: "1"},
				{QuestionCode: "A2", CorrectOptionCode: "3"},
			},
		}},
		"total",
	)
	if got.Primary == nil || got.Primary.Value != 1 || got.Primary.Max == nil || *got.Primary.Max != 2 {
		t.Fatalf("primary = %#v, want 1/2", got.Primary)
	}
	if len(got.Dimensions) != 2 {
		t.Fatalf("dimensions = %#v", got.Dimensions)
	}
	if got.Dimensions[0].Role != "task_set" || got.Dimensions[0].Score == nil || got.Dimensions[0].Score.Value != 1 {
		t.Fatalf("set dimension = %#v", got.Dimensions[0])
	}
	if got.Dimensions[1].Code != "total" || got.Dimensions[1].Role != "total" || got.Dimensions[1].Kind != calculation.DimensionKindAbility {
		t.Fatalf("total dimension = %#v", got.Dimensions[1])
	}
}

func TestScoreSPMAllCorrect(t *testing.T) {
	t.Parallel()
	got := ScoreSPM(
		map[string]string{"A1": "1", "A2": "3"},
		[]ItemSet{{
			Code: "A",
			Items: []Item{
				{QuestionCode: "A1", CorrectOptionCode: "1"},
				{QuestionCode: "A2", CorrectOptionCode: "3"},
			},
		}},
		"total",
	)
	if got.Primary == nil || got.Primary.Value != 2 {
		t.Fatalf("primary = %#v, want 2", got.Primary)
	}
}

func TestScoreSPMUnansweredCountsZero(t *testing.T) {
	t.Parallel()
	got := ScoreSPM(
		nil,
		[]ItemSet{{
			Code:  "A",
			Items: []Item{{QuestionCode: "A1", CorrectOptionCode: "1"}},
		}},
		"total",
	)
	if got.Primary == nil || got.Primary.Value != 0 || got.Primary.Max == nil || *got.Primary.Max != 1 {
		t.Fatalf("primary = %#v, want 0/1", got.Primary)
	}
}

func TestScoreSPMEmptySets(t *testing.T) {
	t.Parallel()
	got := ScoreSPM(map[string]string{"A1": "1"}, nil, "total")
	if got.Primary == nil || got.Primary.Value != 0 {
		t.Fatalf("primary = %#v, want 0", got.Primary)
	}
	if len(got.Dimensions) != 1 || got.Dimensions[0].Code != "total" {
		t.Fatalf("dimensions = %#v", got.Dimensions)
	}
}

func TestScoreSPMOmitsTotalWhenFactorCodeEmpty(t *testing.T) {
	t.Parallel()
	got := ScoreSPM(
		map[string]string{"A1": "1"},
		[]ItemSet{{Code: "A", Items: []Item{{QuestionCode: "A1", CorrectOptionCode: "1"}}}},
		"",
	)
	if len(got.Dimensions) != 1 || got.Dimensions[0].Role != "task_set" {
		t.Fatalf("dimensions = %#v, want only task_set", got.Dimensions)
	}
}
