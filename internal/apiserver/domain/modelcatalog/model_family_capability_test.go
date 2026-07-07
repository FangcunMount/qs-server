package modelcatalog

import "testing"

func TestModelFamilyCapabilitiesV2ExcludeProductChannel(t *testing.T) {
	t.Parallel()

	caps := ModelFamilyCapabilitiesV2()
	if len(caps) != 5 {
		t.Fatalf("model family capability count = %d, want 5", len(caps))
	}
	for _, cap := range caps {
		if cap.Kind == KindBehaviorAbility {
			t.Fatalf("product channel leaked into model families: %#v", cap)
		}
	}
}

func TestModelFamilyCapabilityByKind(t *testing.T) {
	t.Parallel()

	if _, ok := ModelFamilyCapabilityByKind(KindBehaviorAbility); ok {
		t.Fatal("behavior_ability must not resolve as model family capability")
	}
	cap, ok := ModelFamilyCapabilityByKind(KindPersonality)
	if !ok || cap.Kind != KindPersonality || !cap.RuntimeExecutable {
		t.Fatalf("personality model family capability = %#v, %v", cap, ok)
	}
}

func TestModelFamilyCapabilityAllowsOperations(t *testing.T) {
	t.Parallel()

	personality, ok := ModelFamilyCapabilityByKind(KindPersonality)
	if !ok {
		t.Fatal("ModelFamilyCapabilityByKind(personality) = false")
	}
	for _, op := range []CatalogOperation{
		CatalogOpCreate,
		CatalogOpPublish,
		CatalogOpBindQuestionnaire,
	} {
		if !personality.Allows(op) {
			t.Fatalf("personality.Allows(%s) = false", op)
		}
	}
	if personality.Allows(CatalogOpPreview) || personality.Allows(CatalogOpQRCode) {
		t.Fatal("model family capability must not allow preview/qrcode operations")
	}
}
