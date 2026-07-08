package evaluation

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestPersonalityTypologyIdentity(t *testing.T) {
	got := PersonalityTypologyIdentity(modelcatalog.AlgorithmMBTI)
	if got != ExecutionIdentityMBTI {
		t.Fatalf("identity = %#v, want %#v", got, ExecutionIdentityMBTI)
	}
	if got.String() != "typology/typology/mbti" {
		t.Fatalf("identity string = %s", got.String())
	}
}

func TestExecutionIdentityPersonalityTypology(t *testing.T) {
	if ExecutionIdentityPersonalityTypology.String() != "typology/typology/personality_typology" {
		t.Fatalf("identity string = %s", ExecutionIdentityPersonalityTypology.String())
	}
	if !ExecutionIdentityMBTI.IsPersonalityTypologyLegacyIdentity() {
		t.Fatal("mbti identity should be legacy typology alias")
	}
	if ResolvePersonalityTypologyExecutorIdentity(ExecutionIdentityMBTI) != ExecutionIdentityPersonalityTypology {
		t.Fatalf("resolved identity = %#v", ResolvePersonalityTypologyExecutorIdentity(ExecutionIdentityMBTI))
	}
}

func TestPersonalityTypologyLegacyIdentitiesStayFrozen(t *testing.T) {
	want := []ExecutionIdentity{
		ExecutionIdentityMBTI,
		ExecutionIdentitySBTI,
		ExecutionIdentityBigFive,
	}
	got := PersonalityTypologyLegacyIdentities()
	if len(got) != len(want) {
		t.Fatalf("legacy identities = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("legacy identities[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
