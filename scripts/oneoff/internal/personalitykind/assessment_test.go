package personalitykind

import "testing"

func TestEvaluateAssessmentPersonalityKindRewrite(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind, algorithm string
		eligible        bool
		reason          string
	}{
		{"personality", "mbti", true, "personality_kind_to_typology"},
		{"personality", "sbti", true, "personality_kind_to_typology"},
		{"personality", "bigfive", true, "personality_kind_to_typology"},
		{"personality", "personality_typology", true, "personality_kind_to_typology"},
		{"personality", "", true, "personality_kind_to_typology"},
		{"personality", "brief2", false, "non_typology_algorithm"},
		{"typology", "mbti", false, "not_legacy_personality_kind"},
		{"scale", "", false, "not_legacy_personality_kind"},
	}
	for _, tc := range cases {
		got := EvaluateAssessmentPersonalityKindRewrite(tc.kind, tc.algorithm)
		if got.Eligible != tc.eligible || got.Reason != tc.reason {
			t.Fatalf("%s/%s = eligible=%v reason=%s, want eligible=%v reason=%s",
				tc.kind, tc.algorithm, got.Eligible, got.Reason, tc.eligible, tc.reason)
		}
		if got.Eligible && (got.ToKind != "typology" || got.ToSubKind != "typology") {
			t.Fatalf("target = %#v", got)
		}
	}
}
