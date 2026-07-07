package identity

import "testing"

func TestDefaultProductChannelFor(t *testing.T) {
	tests := []struct {
		kind Kind
		want ProductChannel
	}{
		{KindScale, ProductChannelMedicalScale},
		{KindPersonality, ProductChannelPersonality},
		{KindBehavioralRating, ProductChannelBehaviorAbility},
		{KindCognitive, ProductChannelCognitive},
		{KindCustom, ProductChannelCustom},
	}
	for _, tc := range tests {
		if got := DefaultProductChannelFor(tc.kind); got != tc.want {
			t.Fatalf("DefaultProductChannelFor(%s) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestCompleteProductChannel(t *testing.T) {
	got, err := CompleteProductChannel(KindBehavioralRating, ProductChannelMedicalScale)
	if err != nil {
		t.Fatalf("CompleteProductChannel: %v", err)
	}
	if got != ProductChannelMedicalScale {
		t.Fatalf("got %q, want medical_scale", got)
	}

	if _, err := CompleteProductChannel(KindBehavioralRating, ProductChannel("invalid")); err == nil {
		t.Fatal("expected invalid product channel error")
	}

	got, err = CompleteProductChannel(KindBehavioralRating, "")
	if err != nil {
		t.Fatalf("CompleteProductChannel default: %v", err)
	}
	if got != ProductChannelBehaviorAbility {
		t.Fatalf("default channel = %q, want behavior_ability", got)
	}
}
