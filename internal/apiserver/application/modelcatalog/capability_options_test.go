package modelcatalog

import "testing"

func TestAPIKindOptionsExposeOnlyCanonicalKinds(t *testing.T) {
	t.Parallel()

	got := apiKindOptions()
	want := []Option{
		{Label: "医学量表", Value: KindScale},
		{Label: "人格测评", Value: KindTypology},
		{Label: "行为评分", Value: KindBehavioralRating},
		{Label: "认知测评", Value: KindCognitive},
	}
	if len(got) != len(want) {
		t.Fatalf("apiKindOptions() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("apiKindOptions()[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
