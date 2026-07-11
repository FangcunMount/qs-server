package report

import "testing"

func TestDraftCopiesContent(t *testing.T) {
	max := 10.0
	draft := NewDraft(Content{
		PrimaryScore: NewRawTotalScore(7, &max),
		Suggestions:  []Suggestion{{Content: "first"}},
	})
	content := draft.Content()
	*content.PrimaryScore.Max = 20
	content.Suggestions[0].Content = "changed"

	again := draft.Content()
	if *again.PrimaryScore.Max != 10 {
		t.Fatalf("max = %v, want immutable copy", *again.PrimaryScore.Max)
	}
	if again.Suggestions[0].Content != "first" {
		t.Fatalf("suggestion = %q, want immutable copy", again.Suggestions[0].Content)
	}
}
