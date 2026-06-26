package evaluation

import "testing"

func TestDefaultModelDescriptorsReturnsScaleOnly(t *testing.T) {
	descs := DefaultModelDescriptors()
	if len(descs) != 1 {
		t.Fatalf("descriptor count = %d, want 1", len(descs))
	}
	if descs[0].Kind != ModelKindScale {
		t.Fatalf("descriptor kind = %s, want %s", descs[0].Kind, ModelKindScale)
	}
	if len(TypologyAlgorithms(descs)) != 0 {
		t.Fatalf("typology algorithms = %#v, want empty", TypologyAlgorithms(descs))
	}
}
