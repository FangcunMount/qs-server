package option

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

func TestBehaviorAbilityChannelAggregatesOnlyModelFamilies(t *testing.T) {
	t.Parallel()

	families := BehaviorAbilityChannelModelFamilies()
	if len(families) != 2 || families[0] != binding.KindBehavioralRating || families[1] != binding.KindCognitive {
		t.Fatalf("families = %#v", families)
	}
	if !IsBehaviorAbilityProductChannelAPIKind(APIKindBehaviorAbility) {
		t.Fatal("behavior_ability must remain an API product channel")
	}
	if kind, ok := ResolveBehaviorAbilityChannelFamily("cognitive"); !ok || kind != binding.KindCognitive {
		t.Fatalf("resolved family = %q, %v", kind, ok)
	}
	if _, ok := ResolveBehaviorAbilityChannelFamily(APIKindBehaviorAbility); ok {
		t.Fatal("product channel must not be resolved as a model family")
	}
}
