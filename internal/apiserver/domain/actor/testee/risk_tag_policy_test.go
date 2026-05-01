package testee

import "testing"

func TestRiskTagPolicyReplacesAssessmentRiskTags(t *testing.T) {
	item := NewTestee(1, "testee", GenderUnknown, nil)
	item.SetTags([]Tag{TagRiskHigh, TagRiskSevere, Tag("manual")})
	policy := NewRiskTagPolicy()

	decision, err := policy.ApplyAssessmentResult(item, "medium", false)
	if err != nil {
		t.Fatalf("ApplyAssessmentResult returned error: %v", err)
	}

	assertTags(t, decision.TagsRemoved, []Tag{TagRiskHigh, TagRiskSevere})
	assertTags(t, decision.TagsAdded, []Tag{TagRiskMedium})
	assertTags(t, item.Tags(), []Tag{Tag("manual"), TagRiskMedium})
	if decision.KeyFocusMarked {
		t.Fatalf("expected key focus to remain false")
	}
}

func TestRiskTagPolicySevereRiskAddsHighAndSevereTags(t *testing.T) {
	item := NewTestee(1, "testee", GenderUnknown, nil)
	policy := NewRiskTagPolicy()

	decision, err := policy.ApplyAssessmentResult(item, "severe", true)
	if err != nil {
		t.Fatalf("ApplyAssessmentResult returned error: %v", err)
	}

	assertTags(t, decision.TagsAdded, []Tag{TagRiskHigh, TagRiskSevere})
	assertTags(t, item.Tags(), []Tag{TagRiskHigh, TagRiskSevere})
	if !decision.KeyFocusMarked || !item.IsKeyFocus() {
		t.Fatalf("expected severe risk with markKeyFocus to mark key focus")
	}
}

func TestRiskTagPolicyUnmarksKeyFocusForNonHighRiskWhenRequested(t *testing.T) {
	item := NewTestee(1, "testee", GenderUnknown, nil)
	item.SetKeyFocus(true)
	policy := NewRiskTagPolicy()

	decision, err := policy.ApplyAssessmentResult(item, "low", false)
	if err != nil {
		t.Fatalf("ApplyAssessmentResult returned error: %v", err)
	}

	if decision.KeyFocusMarked || item.IsKeyFocus() {
		t.Fatalf("expected non-high risk with markKeyFocus=false to unmark key focus")
	}
}

func assertTags(t *testing.T, actual, expected []Tag) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("expected tags %v, got %v", expected, actual)
	}
	for i := range expected {
		if actual[i] != expected[i] {
			t.Fatalf("expected tags %v, got %v", expected, actual)
		}
	}
}
