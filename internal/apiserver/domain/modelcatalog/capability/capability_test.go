package capability

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestFamilyCapabilityByKind(t *testing.T) {
	t.Parallel()

	if _, ok := FamilyCapabilityByKind(identity.Kind("unknown")); ok {
		t.Fatal("FamilyCapabilityByKind(unknown) = true, want false")
	}
	cap, ok := FamilyCapabilityByKind(identity.KindPersonality)
	if !ok || !cap.RuntimeExecutable {
		t.Fatalf("FamilyCapabilityByKind(personality) = %#v, %v", cap, ok)
	}
}

func TestRuntimeExecutableKinds(t *testing.T) {
	t.Parallel()

	kinds := RuntimeExecutableKinds()
	if len(kinds) < 4 {
		t.Fatalf("RuntimeExecutableKinds() = %#v, want executable families", kinds)
	}
}
