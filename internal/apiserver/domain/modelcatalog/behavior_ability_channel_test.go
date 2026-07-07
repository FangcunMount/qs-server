package modelcatalog

import "testing"

func TestBehaviorAbilityChannelFamilies(t *testing.T) {
	t.Parallel()

	families := BehaviorAbilityChannelModelFamilies()
	if len(families) != 2 || families[0] != KindBehavioralRating || families[1] != KindCognitive {
		t.Fatalf("families = %#v", families)
	}
	if !IsBehaviorAbilityProductChannelAPIKind(APIKindBehaviorAbility) {
		t.Fatal("behavior_ability must be a product channel api kind")
	}
	for _, kind := range []Kind{KindBehavioralRating, KindCognitive, KindBehaviorAbility} {
		if !IsBehaviorAbilityChannelFamily(kind) {
			t.Fatalf("%q must belong to behavior_ability channel", kind)
		}
	}
	if IsBehaviorAbilityChannelFamily(KindPersonality) {
		t.Fatal("personality must not belong to behavior_ability channel")
	}
}

func TestResolveBehaviorAbilityChannelFamily(t *testing.T) {
	t.Parallel()

	kind, ok := ResolveBehaviorAbilityChannelFamily(string(KindBehavioralRating))
	if !ok || kind != KindBehavioralRating {
		t.Fatalf("resolve behavioral_rating = %q, %v", kind, ok)
	}
	if _, ok := ResolveBehaviorAbilityChannelFamily("personality"); ok {
		t.Fatal("personality must not resolve as channel family filter")
	}
}
