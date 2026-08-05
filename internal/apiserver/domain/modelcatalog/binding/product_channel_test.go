package binding

import "testing"

func TestDefaultProductChannelFor(t *testing.T) {
	tests := []struct {
		kind Kind
		want ProductChannel
	}{
		{KindScale, ProductChannelMedicalScale},
		{KindTypology, ProductChannelTypology},
		{KindBehavioralRating, ProductChannelBehaviorAbility},
		{KindCognitive, ProductChannelBehaviorAbility},
	}
	for _, tc := range tests {
		if got := DefaultProductChannelFor(tc.kind); got != tc.want {
			t.Fatalf("DefaultProductChannelFor(%s) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}
