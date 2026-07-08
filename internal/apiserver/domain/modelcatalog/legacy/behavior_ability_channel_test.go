package legacy

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

func TestBehaviorAbilityChannelFamilies(t *testing.T) {
	t.Parallel()

	families := BehaviorAbilityChannelModelFamilies()
	if len(families) != 2 || families[0] != binding.KindBehavioralRating || families[1] != binding.KindCognitive {
		t.Fatalf("families = %#v", families)
	}
	if !IsBehaviorAbilityProductChannelAPIKind(APIKindBehaviorAbility) {
		t.Fatal("behavior_ability must be a product channel api kind")
	}
	for _, kind := range []binding.Kind{binding.KindBehavioralRating, binding.KindCognitive} {
		if !IsBehaviorAbilityChannelFamily(kind) {
			t.Fatalf("%q must belong to behavior_ability channel", kind)
		}
	}
	if IsBehaviorAbilityChannelFamily(binding.KindPersonality) {
		t.Fatal("personality must not belong to behavior_ability channel")
	}
}

func TestResolveBehaviorAbilityChannelFamily(t *testing.T) {
	t.Parallel()

	kind, ok := ResolveBehaviorAbilityChannelFamily(string(binding.KindBehavioralRating))
	if !ok || kind != binding.KindBehavioralRating {
		t.Fatalf("resolve behavioral_rating = %q, %v", kind, ok)
	}
	if _, ok := ResolveBehaviorAbilityChannelFamily("personality"); ok {
		t.Fatal("personality must not resolve as channel family filter")
	}
}
