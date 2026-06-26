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

func TestEvaluatorKeyPersonalityTypology(t *testing.T) {
	if EvaluatorKeyPersonalityTypology.String() != "personality/typology/personality_typology" {
		t.Fatalf("key string = %s", EvaluatorKeyPersonalityTypology.String())
	}
	if !EvaluatorKeyMBTI.IsPersonalityTypologyLegacyKey() {
		t.Fatal("mbti key should be legacy typology alias")
	}
	if ResolvePersonalityTypologyExecutorKey(EvaluatorKeyMBTI) != EvaluatorKeyPersonalityTypology {
		t.Fatalf("resolved key = %#v", ResolvePersonalityTypologyExecutorKey(EvaluatorKeyMBTI))
	}
}
