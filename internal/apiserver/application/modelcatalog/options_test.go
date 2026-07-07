package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestModelCatalogOptionsIncludeLegacyBehaviorAbility(t *testing.T) {
	t.Parallel()

	var found bool
	for _, opt := range ModelCatalogOptions() {
		if opt.APIKind == KindBehaviorAbility {
			found = true
			if opt.ProductChannel != domain.KindBehaviorAbility { //nolint:staticcheck // SA1019: behavior_ability legacy product-channel compatibility
				t.Fatalf("behavior_ability option channel = %q, want behavior_ability", opt.ProductChannel)
			}
			if !opt.OptionsEnabled {
				t.Fatal("behavior_ability option must remain selectable for legacy API")
			}
		}
	}
	if !found {
		t.Fatal("ModelCatalogOptions missing legacy behavior_ability channel")
	}
}

func TestModelCatalogOptionsExcludeProductChannelFromModelFamilies(t *testing.T) {
	t.Parallel()

	for _, cap := range domain.ModelFamilyCapabilities() {
		if cap.Kind == domain.KindBehaviorAbility { //nolint:staticcheck // SA1019: behavior_ability legacy product-channel compatibility
			t.Fatalf("product channel leaked into model families: %#v", cap)
		}
	}
}
