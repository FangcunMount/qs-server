package modelcatalog

import "testing"

func TestAPIKindOptionsMarksReservedKindsDisabled(t *testing.T) {
	t.Parallel()

	disabled := make(map[string]bool)
	for _, opt := range apiKindOptions() {
		if opt.Disabled {
			disabled[opt.Value] = true
		}
	}
	for _, apiKind := range []string{KindCustom} {
		if !disabled[apiKind] {
			t.Fatalf("api kind option %q must be disabled", apiKind)
		}
	}
}
