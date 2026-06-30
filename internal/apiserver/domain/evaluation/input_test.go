package evaluation_test

import (
	"testing"

	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

func TestAnswerValueKeyUnwrapsOptionWrapper(t *testing.T) {
	t.Parallel()

	if got := evaluationinput.AnswerValueKey(`{"option":"5"}`); got != "5" {
		t.Fatalf("AnswerValueKey() = %q, want 5", got)
	}
	if got := evaluationinput.AnswerValueKey(map[string]any{"option": "A"}); got != "A" {
		t.Fatalf("AnswerValueKey() = %q, want A", got)
	}
}
