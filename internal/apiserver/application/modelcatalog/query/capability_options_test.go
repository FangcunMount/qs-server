package query

import (
	"testing"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
)

func TestAPIKindOptionsExposeOnlyCanonicalKinds(t *testing.T) {
	t.Parallel()

	got := apiKindOptions()
	want := []modelcatalog.Option{
		{Label: "医学量表", Value: modelcatalog.KindScale},
		{Label: "人格测评", Value: modelcatalog.KindTypology},
		{Label: "行为评分", Value: modelcatalog.KindBehavioralRating},
		{Label: "认知测评", Value: modelcatalog.KindCognitive},
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
