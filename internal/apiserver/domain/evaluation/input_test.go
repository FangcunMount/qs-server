package evaluation_test

import (
	"testing"

	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
)

func TestAnswerValueKeyUnwrapsOptionWrapper(t *testing.T) {
	t.Parallel()

	if got := evalinput.AnswerValueKey(`{"option":"5"}`); got != "5" {
		t.Fatalf("AnswerValueKey() = %q, want 5", got)
	}
	if got := evalinput.AnswerValueKey(map[string]any{"option": "A"}); got != "A" {
		t.Fatalf("AnswerValueKey() = %q, want A", got)
	}
}
