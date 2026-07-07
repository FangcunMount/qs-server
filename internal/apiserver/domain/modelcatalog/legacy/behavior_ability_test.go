package legacy

import "testing"

func TestBehaviorAbilityProductChannelAPIKind(t *testing.T) {
	t.Parallel()

	if APIKindBehaviorAbility != "behavior_ability" {
		t.Fatalf("APIKindBehaviorAbility = %q", APIKindBehaviorAbility)
	}
	if !IsBehaviorAbilityProductChannelAPIKind(APIKindBehaviorAbility) {
		t.Fatal("behavior_ability must remain the product-channel API kind")
	}
}
