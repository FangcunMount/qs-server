package binding

import "testing"

func TestFamilyCapabilityByKindSeparatesDomainGuards(t *testing.T) {
	t.Parallel()

	family, ok := FamilyCapabilityByKind(KindTypology)
	if !ok || !family.CreateSupported || !family.RuntimeExecutable {
		t.Fatalf("personality family capability = %#v, %v", family, ok)
	}
}
