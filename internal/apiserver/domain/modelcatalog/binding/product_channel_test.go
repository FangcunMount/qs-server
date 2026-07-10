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

func TestCompleteProductChannel(t *testing.T) {
	got, err := CompleteProductChannel(KindBehavioralRating, ProductChannelMedicalScale)
	if err != nil {
		t.Fatalf("CompleteProductChannel: %v", err)
	}
	if got != ProductChannelMedicalScale {
		t.Fatalf("got %q, want medical_scale", got)
	}

	if _, err := CompleteProductChannel(KindCognitive, ProductChannel("cognitive")); err == nil {
		t.Fatal("expected removed cognitive channel alias to be rejected")
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

func TestAllProductChannelsOnlyReturnsProductConcepts(t *testing.T) {
	got := AllProductChannels()
	want := []ProductChannel{
		ProductChannelMedicalScale,
		ProductChannelTypology,
		ProductChannelBehaviorAbility,
	}
	if len(got) != len(want) {
		t.Fatalf("AllProductChannels() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AllProductChannels()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestProductFromChannelNormalizesToThreeProducts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		channel ProductChannel
		want    Product
	}{
		{ProductChannelMedicalScale, ProductMedicalScale},
		{ProductChannelTypology, ProductTypology},
		{ProductChannelBehaviorAbility, ProductBehaviorAbility},
	}
	for _, tc := range cases {
		got, err := ProductFromChannel(tc.channel)
		if err != nil {
			t.Fatalf("ProductFromChannel(%q): %v", tc.channel, err)
		}
		if got != tc.want {
			t.Fatalf("ProductFromChannel(%q) = %q, want %q", tc.channel, got, tc.want)
		}
	}
}
