package evaluation

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestPersonalityTypologyIdentity(t *testing.T) {
	got := PersonalityTypologyIdentity(modelcatalog.AlgorithmMBTI)
	if got.String() != "typology/typology/mbti" {
		t.Fatalf("identity string = %s", got.String())
	}
}

func TestExecutionIdentityPersonalityTypology(t *testing.T) {
	if ExecutionIdentityPersonalityTypology.String() != "typology/typology/personality_typology" {
		t.Fatalf("identity string = %s", ExecutionIdentityPersonalityTypology.String())
	}
}
