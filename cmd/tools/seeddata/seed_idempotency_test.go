package main

import "testing"

func TestSameStringSet(t *testing.T) {
	if !sameStringSet([]string{"qs:staff", "qs:evaluator"}, []string{"qs:evaluator", "qs:staff"}) {
		t.Fatal("expected sameStringSet to ignore ordering")
	}
	if sameStringSet([]string{"qs:staff"}, []string{"qs:evaluator"}) {
		t.Fatal("expected sameStringSet to detect different sets")
	}
}

func TestFilterAssessmentTargetsForBackfill(t *testing.T) {
	counters := newAssessmentCounters()
	targets := []scaleTarget{
		{QuestionnaireCode: "Q1"},
		{QuestionnaireCode: "Q2"},
		{QuestionnaireCode: "Q3"},
	}
	existing := map[string]struct{}{
		"Q2": {},
	}

	filtered := filterAssessmentTargetsForBackfill(targets, existing, counters)
	if len(filtered) != 2 {
		t.Fatalf("unexpected filtered target count: got=%d want=2", len(filtered))
	}
	if filtered[0].QuestionnaireCode != "Q1" || filtered[1].QuestionnaireCode != "Q3" {
		t.Fatalf("unexpected filtered targets: %+v", filtered)
	}
	if got := counters.Snapshot().skippedCount; got != 1 {
		t.Fatalf("unexpected skipped count: got=%d want=1", got)
	}
	if _, ok := existing["Q1"]; !ok {
		t.Fatal("expected Q1 to be recorded as existing after filtering")
	}
	if _, ok := existing["Q3"]; !ok {
		t.Fatal("expected Q3 to be recorded as existing after filtering")
	}
}

func TestNewAssessmentTesteeRandIsDeterministic(t *testing.T) {
	left := newAssessmentTesteeRand("12345")
	right := newAssessmentTesteeRand("12345")
	for i := 0; i < 5; i++ {
		if left.Int63() != right.Int63() {
			t.Fatal("expected deterministic random stream for same testee id")
		}
	}
}
