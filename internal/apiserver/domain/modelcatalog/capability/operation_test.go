package capability

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestModelFamilyCapabilityAllowsOperations(t *testing.T) {
	t.Parallel()

	personality, ok := FamilyCapabilityByKind(identity.KindPersonality)
	if !ok {
		t.Fatal("missing personality capability")
	}
	for _, op := range []CatalogOperation{
		CatalogOpCreate,
		CatalogOpList,
		CatalogOpPublish,
		CatalogOpBindQuestionnaire,
		CatalogOpUpdateDefinition,
	} {
		if !personality.Allows(op) {
			t.Fatalf("personality should allow %s", op)
		}
	}
	for _, kind := range []identity.Kind{
		identity.KindScale,
		identity.KindCustom,
	} {
		cap, ok := FamilyCapabilityByKind(kind)
		if !ok {
			t.Fatalf("missing capability for %s", kind)
		}
		if cap.Allows(CatalogOpCreate) {
			t.Fatalf("%s create should be rejected", kind)
		}
	}
}
