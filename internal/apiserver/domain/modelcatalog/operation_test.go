package modelcatalog

import "testing"

func TestKindCapabilityAllowsOperations(t *testing.T) {
	t.Parallel()

	personality, ok := CapabilityByKind(KindPersonality)
	if !ok {
		t.Fatal("personality capability missing")
	}
	for _, op := range []CatalogOperation{
		CatalogOpCreate,
		CatalogOpList,
		CatalogOpUpdateBasicInfo,
		CatalogOpDelete,
		CatalogOpPublish,
		CatalogOpUnpublish,
		CatalogOpArchive,
		CatalogOpBindQuestionnaire,
		CatalogOpUpdateDefinition,
		CatalogOpPreview,
		CatalogOpQRCode,
	} {
		if !personality.Allows(op) {
			t.Fatalf("personality must allow %q", op)
		}
	}

	behavior, ok := CapabilityByKind(KindBehaviorAbility)
	if !ok {
		t.Fatal("behavior_ability capability missing")
	}
	if !behavior.Allows(CatalogOpUpdateDefinition) {
		t.Fatal("behavior_ability must allow definition update")
	}
	if behavior.Allows(CatalogOpPreview) {
		t.Fatal("behavior_ability must not allow preview")
	}

	for _, kind := range []Kind{KindScale, KindCustom} {
		cap, ok := CapabilityByKind(kind)
		if !ok {
			t.Fatalf("capability missing for %q", kind)
		}
		for _, op := range []CatalogOperation{
			CatalogOpCreate,
			CatalogOpUpdateDefinition,
			CatalogOpPublish,
			CatalogOpBindQuestionnaire,
		} {
			if cap.Allows(op) {
				t.Fatalf("%s must not allow %q", kind, op)
			}
		}
	}
}
