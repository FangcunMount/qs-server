package binding

import "testing"

func TestFamilyCapabilityByKindReturnsRuntimeGuard(t *testing.T) {
	t.Parallel()

	family, ok := FamilyCapabilityByKind(KindTypology)
	if !ok || !family.RuntimeExecutable {
		t.Fatalf("personality family capability = %#v, %v", family, ok)
	}
}
