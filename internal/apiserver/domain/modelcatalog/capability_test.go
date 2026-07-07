package modelcatalog

import "testing"

func TestDefaultCapabilitiesMatrix(t *testing.T) {
	t.Parallel()

	caps := DefaultCapabilities()
	if len(caps) != 6 {
		t.Fatalf("capability count = %d, want 6", len(caps))
	}

	byKind := make(map[Kind]KindCapability, len(caps))
	for _, cap := range caps {
		byKind[cap.Kind] = cap
	}

	personality := byKind[KindPersonality]
	if !personality.CreateSupported || !personality.PreviewSupported || !personality.RuntimeExecutable {
		t.Fatalf("personality capability = %#v", personality)
	}

	behaviorAbility := byKind[KindBehaviorAbility]
	if behaviorAbility.CreateSupported || behaviorAbility.PreviewSupported || behaviorAbility.RuntimeExecutable {
		t.Fatalf("behavior_ability capability = %#v", behaviorAbility)
	}
	if behaviorAbility.Role != CapabilityRoleProductChannel || !behaviorAbility.IsProductChannel() {
		t.Fatalf("behavior_ability role = %q, IsProductChannel = %v", behaviorAbility.Role, behaviorAbility.IsProductChannel())
	}
	if behaviorAbility.AllowsNewDraft() {
		t.Fatal("behavior_ability must not allow new drafts")
	}
	if !behaviorAbility.DefinitionUpdateSupported {
		t.Fatal("behavior_ability must allow definition update")
	}
	if !behaviorAbility.RuntimeViaScaleLegacy || !behaviorAbility.CanExecute() {
		t.Fatalf("behavior_ability must execute via scale legacy binding")
	}

	behavioralRating := byKind[KindBehavioralRating]
	if !behavioralRating.CreateSupported || behavioralRating.RuntimeViaScaleLegacy || !behavioralRating.RuntimeExecutable {
		t.Fatalf("behavioral_rating capability = %#v", behavioralRating)
	}
	if behavioralRating.Role != CapabilityRoleModelFamily || !behavioralRating.AllowsNewDraft() {
		t.Fatalf("behavioral_rating role/guard = %#v", behavioralRating)
	}

	scale := byKind[KindScale]
	if scale.CreateSupported || !scale.RuntimeExecutable {
		t.Fatalf("scale capability = %#v", scale)
	}

	for _, kind := range []Kind{KindCognitive, KindCustom} {
		cap := byKind[kind]
		if kind == KindCustom {
			if cap.OptionsEnabled || cap.CreateSupported || cap.CanExecute() {
				t.Fatalf("%s capability = %#v, want catalog-only disabled family", kind, cap)
			}
			continue
		}
		if !cap.OptionsEnabled || !cap.CreateSupported || !cap.CanExecute() {
			t.Fatalf("%s capability = %#v, want enabled cognitive family", kind, cap)
		}
	}
}

func TestCapabilityByKind(t *testing.T) {
	t.Parallel()

	if _, ok := CapabilityByKind(Kind("unknown")); ok {
		t.Fatal("CapabilityByKind(unknown) = true, want false")
	}

	cap, ok := CapabilityByKind(KindPersonality)
	if !ok || cap.Kind != KindPersonality {
		t.Fatalf("CapabilityByKind(personality) = %#v, %v", cap, ok)
	}
}

func TestReservedKindsAreNotRuntimeExecutable(t *testing.T) {
	t.Parallel()

	executable := make(map[Kind]bool, len(RuntimeExecutableKinds()))
	for _, kind := range RuntimeExecutableKinds() {
		executable[kind] = true
	}
	if executable[KindCustom] {
		t.Fatal("custom must not be runtime executable")
	}
	cap, ok := CapabilityByKind(KindCustom)
	if !ok {
		t.Fatal("CapabilityByKind(custom) = false")
	}
	if cap.ExecutionPath != ExecutionPathNone {
		t.Fatalf("custom execution path = %q, want none", cap.ExecutionPath)
	}
}

func TestRuntimeExecutableKinds(t *testing.T) {
	t.Parallel()

	got := RuntimeExecutableKinds()
	want := map[Kind]bool{
		KindScale:            true,
		KindPersonality:      true,
		KindBehavioralRating: true,
		KindCognitive:        true,
	}
	if len(got) != len(want) {
		t.Fatalf("RuntimeExecutableKinds() = %#v, want scale + personality + behavioral_rating + cognitive", got)
	}
	for _, kind := range got {
		if !want[kind] {
			t.Fatalf("unexpected runtime executable kind %q", kind)
		}
	}
}
