package legacy

import "testing"

func TestIsDeprecatedProductChannelKind(t *testing.T) {
	t.Parallel()

	if !IsDeprecatedProductChannelKind(KindBehaviorAbilityLegacy) {
		t.Fatal("behavior_ability should be a deprecated product channel kind")
	}
	if IsDeprecatedProductChannelKind("behavioral_rating") {
		t.Fatal("behavioral_rating is not a deprecated product channel kind")
	}
}
