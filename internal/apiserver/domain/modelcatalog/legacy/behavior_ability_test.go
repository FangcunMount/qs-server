package legacy

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

func TestBehaviorAbilityNamingBoundary(t *testing.T) {
	t.Parallel()

	if APIKindBehaviorAbility != "behavior_ability" {
		t.Fatalf("APIKindBehaviorAbility = %q", APIKindBehaviorAbility)
	}
	if PayloadFormatBehaviorAbilityScaleV1 != "assessmentmodel.behavior_ability.scale.v1" {
		t.Fatalf("PayloadFormatBehaviorAbilityScaleV1 = %q", PayloadFormatBehaviorAbilityScaleV1)
	}
	if PayloadFormatBehaviorAbilityScaleV1 == routing.PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatal("behavior_ability scale payload must not alias behavioral_rating.default format")
	}

	if !IsBehaviorAbilityScaleAdapter(BehaviorAbilityKind()) {
		t.Fatal("KindBehaviorAbility should be the behavior_ability scale adapter taxonomy slot")
	}
	for _, kind := range []identity.Kind{
		identity.KindScale,
		identity.KindPersonality,
		identity.KindBehavioralRating,
		identity.KindCognitive,
		identity.KindCustom,
	} {
		if IsBehaviorAbilityScaleAdapter(kind) {
			t.Fatalf("IsBehaviorAbilityScaleAdapter(%q) = true, want false", kind)
		}
	}
}
