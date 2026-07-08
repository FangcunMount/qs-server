package binding

import "testing"

func TestModelFamilyCapabilityAllowsOperations(t *testing.T) {
	t.Parallel()

	personality, ok := FamilyCapabilityByKind(KindPersonality)
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
	for _, kind := range []Kind{
		KindScale,
		KindCustom,
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
