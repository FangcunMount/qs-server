package evaluation

import "testing"

func TestIsTypologyModelAcceptsOnlyCanonicalKind(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		model ModelIdentityResponse
		want  bool
	}{
		{name: "canonical", model: ModelIdentityResponse{Kind: typologyModelKind}, want: true},
		{name: "retired personality kind", model: ModelIdentityResponse{Kind: "personality"}, want: false},
		{name: "scale", model: ModelIdentityResponse{Kind: "scale"}, want: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsTypologyModel(tc.model); got != tc.want {
				t.Fatalf("IsTypologyModel(%+v) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}

func TestScaleIdentityDoesNotTreatCanonicalTypologyAsScale(t *testing.T) {
	t.Parallel()

	model := ModelIdentityResponse{Kind: typologyModelKind, Code: "MBTI", Title: "MBTI"}
	if got := scaleCodeFromModel(model); got != "" {
		t.Fatalf("scaleCodeFromModel() = %q, want empty", got)
	}
	if got := scaleNameFromModel(model); got != "" {
		t.Fatalf("scaleNameFromModel() = %q, want empty", got)
	}
}
