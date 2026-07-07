package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestModelCatalogOptionsExcludeProductChannelFromModelFamilies(t *testing.T) {
	t.Parallel()

	for _, cap := range domain.ModelFamilyCapabilities() {
		if cap.IsProductChannel() {
			t.Fatalf("product channel leaked into model families: %#v", cap)
		}
	}
}

func TestIsSupportedAPIKindIncludesBehaviorAbilityChannel(t *testing.T) {
	t.Parallel()

	if !IsSupportedAPIKind(KindBehaviorAbility) {
		t.Fatal("behavior_ability channel kind must remain supported for Options metadata")
	}
}
