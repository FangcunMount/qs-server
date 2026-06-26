package evaluation

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
)

func TestPersonalityTypologyKey(t *testing.T) {
	got := PersonalityTypologyKey(assessmentmodel.AlgorithmMBTI)
	if got != EvaluatorKeyMBTI {
		t.Fatalf("key = %#v, want %#v", got, EvaluatorKeyMBTI)
	}
	if got.String() != "personality/typology/mbti" {
		t.Fatalf("key string = %s", got.String())
	}
}
