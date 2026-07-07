package capability

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

func TestDefaultCapabilitiesMatrix(t *testing.T) {
	t.Parallel()

	caps := DefaultCapabilities()
	if len(caps) != 5 {
		t.Fatalf("capability count = %d, want 5", len(caps))
	}

	byKind := make(map[identity.Kind]KindCapability, len(caps))
	for _, cap := range caps {
		byKind[cap.Kind] = cap
	}

	personality := byKind[identity.KindPersonality]
	if !personality.CreateSupported || !personality.PreviewSupported || !personality.RuntimeExecutable {
		t.Fatalf("personality capability = %#v", personality)
	}

	behavioralRating := byKind[identity.KindBehavioralRating]
	if !behavioralRating.CreateSupported || !behavioralRating.RuntimeExecutable {
		t.Fatalf("behavioral_rating capability = %#v", behavioralRating)
	}
	if behavioralRating.Role != CapabilityRoleModelFamily || !behavioralRating.AllowsNewDraft() {
		t.Fatalf("behavioral_rating role/guard = %#v", behavioralRating)
	}

	scale := byKind[identity.KindScale]
	if scale.CreateSupported || !scale.RuntimeExecutable {
		t.Fatalf("scale capability = %#v", scale)
	}

	for _, kind := range []identity.Kind{identity.KindCognitive, identity.KindCustom} {
		cap := byKind[kind]
		if kind == identity.KindCustom {
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

	if _, ok := CapabilityByKind(identity.Kind("unknown")); ok {
		t.Fatal("CapabilityByKind(unknown) = true, want false")
	}

	cap, ok := CapabilityByKind(identity.KindPersonality)
	if !ok || cap.Kind != identity.KindPersonality {
		t.Fatalf("CapabilityByKind(personality) = %#v, %v", cap, ok)
	}
}

func TestReservedKindsAreNotRuntimeExecutable(t *testing.T) {
	t.Parallel()

	executable := make(map[identity.Kind]bool, len(RuntimeExecutableKinds()))
	for _, kind := range RuntimeExecutableKinds() {
		executable[kind] = true
	}
	if executable[identity.KindCustom] {
		t.Fatal("custom must not be runtime executable")
	}
	cap, ok := CapabilityByKind(identity.KindCustom)
	if !ok {
		t.Fatal("CapabilityByKind(custom) = false")
	}
	if cap.ExecutionPath != routing.ExecutionPathNone {
		t.Fatalf("custom execution path = %q, want none", cap.ExecutionPath)
	}
}

func TestRuntimeExecutableKinds(t *testing.T) {
	t.Parallel()

	got := RuntimeExecutableKinds()
	want := map[identity.Kind]bool{
		identity.KindScale:            true,
		identity.KindPersonality:      true,
		identity.KindBehavioralRating: true,
		identity.KindCognitive:        true,
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

func TestRuntimeExecutableKindsExcludeProductChannel(t *testing.T) {
	t.Parallel()

	for _, kind := range RuntimeExecutableKinds() {
		cap, ok := CapabilityByKind(kind)
		if !ok {
			t.Fatalf("RuntimeExecutableKinds contains unknown kind %q", kind)
		}
		if cap.IsProductChannel() {
			t.Fatalf("product channel kind %q must not be runtime executable", kind)
		}
	}
}

func TestModelFamilyCapabilitiesExcludeProductChannel(t *testing.T) {
	t.Parallel()

	caps := ModelFamilyCapabilities()
	if len(caps) != 5 {
		t.Fatalf("model family capability count = %d, want 5", len(caps))
	}
	for _, cap := range caps {
		if cap.IsProductChannel() {
			t.Fatalf("product channel capability leaked into model families: %#v", cap)
		}
	}
}

func TestModelFamilyCapabilityByKind(t *testing.T) {
	t.Parallel()

	if _, ok := ModelFamilyCapabilityByKind(identity.Kind("behavior_ability")); ok {
		t.Fatal("behavior_ability must not resolve as model family capability")
	}
	cap, ok := ModelFamilyCapabilityByKind(identity.KindPersonality)
	if !ok || cap.Kind != identity.KindPersonality || !cap.RuntimeExecutable {
		t.Fatalf("personality model family capability = %#v, %v", cap, ok)
	}
}
