package testee

import (
	"context"
	"testing"
)

func TestTaggerKeepsManualAuxiliaryTagsWorking(t *testing.T) {
	item := NewTestee(1, "testee", GenderUnknown, nil)
	tagger := NewTagger(NewValidator(nil))

	if err := tagger.Tag(context.Background(), item, Tag("manual_follow")); err != nil {
		t.Fatalf("Tag returned error: %v", err)
	}
	if err := tagger.Tag(context.Background(), item, Tag("manual_follow")); err != nil {
		t.Fatalf("idempotent Tag returned error: %v", err)
	}
	if got := item.TagsAsStrings(); len(got) != 1 || got[0] != "manual_follow" {
		t.Fatalf("tags = %v, want [manual_follow]", got)
	}

	if err := tagger.UnTag(context.Background(), item, Tag("manual_follow")); err != nil {
		t.Fatalf("UnTag returned error: %v", err)
	}
	if got := item.TagsAsStrings(); len(got) != 0 {
		t.Fatalf("tags = %v, want empty", got)
	}
}
