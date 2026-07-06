package modelcatalog

import "testing"

func TestBehaviorAbilityNamingBoundary(t *testing.T) {
	t.Parallel()

	if APIKindBehaviorAbility != "behavior_ability" {
		t.Fatalf("APIKindBehaviorAbility = %q", APIKindBehaviorAbility)
	}
	if PayloadFormatBehaviorAbilityScaleV1 != "assessmentmodel.behavior_ability.scale.v1" {
		t.Fatalf("PayloadFormatBehaviorAbilityScaleV1 = %q", PayloadFormatBehaviorAbilityScaleV1)
	}
	if PayloadFormatBehaviorAbilityScaleV1 == PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatal("behavior_ability scale payload must not alias behavioral_rating.default format")
	}

	if !IsBehaviorAbilityScaleAdapter(KindBehaviorAbility) {
		t.Fatal("KindBehaviorAbility should be the behavior_ability scale adapter taxonomy slot")
	}
	for _, kind := range []Kind{KindScale, KindPersonality, KindBehavioralRating, KindCognitive, KindCustom} {
		if IsBehaviorAbilityScaleAdapter(kind) {
			t.Fatalf("IsBehaviorAbilityScaleAdapter(%q) = true, want false", kind)
		}
	}

	cap, ok := BehaviorAbilityScaleAdapterCapability()
	if !ok {
		t.Fatal("BehaviorAbilityScaleAdapterCapability() = false, want true")
	}
	if cap.APIKind != APIKindBehaviorAbility {
		t.Fatalf("APIKind = %q, want %q", cap.APIKind, APIKindBehaviorAbility)
	}
	if cap.RuntimeExecutable || !cap.RuntimeViaScaleLegacy {
		t.Fatalf("capability = %#v, want scale adapter routing only", cap)
	}
	if cap.ExecutionPath != ExecutionPathBehaviorAbilityScaleAdapter {
		t.Fatalf("ExecutionPath = %q, want behavior_ability_scale_adapter", cap.ExecutionPath)
	}
}
