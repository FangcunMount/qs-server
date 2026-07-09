package modelcatalog

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/option"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestModelCatalogOptionsExcludeProductChannelFromModelFamilies(t *testing.T) {
	t.Parallel()

	for _, entry := range option.DefaultRegistry().RegisteredOptions() {
		if entry.IsProductChannel() {
			continue
		}
		cap, ok := domain.FamilyCapabilityByKind(entry.Kind)
		if !ok {
			continue
		}
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

func TestProductChannelOptionsOnlyExposeProductConcepts(t *testing.T) {
	t.Parallel()

	got := productChannelOptions()
	want := []Option{
		{Label: "医学量表", Value: string(domain.ProductChannelMedicalScale)},
		{Label: "人格测评", Value: string(domain.ProductChannelTypology)},
		{Label: "行为能力", Value: string(domain.ProductChannelBehaviorAbility)},
	}
	if len(got) != len(want) {
		t.Fatalf("productChannelOptions() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("productChannelOptions()[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
