package modelcatalog

import (
	"os"
	"strings"
	"testing"
)

func TestPersonalityDepsUseSeparatePublishedWriteRepository(t *testing.T) {
	content, err := os.ReadFile("wire.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "PublishedRepo:") || !strings.Contains(text, "publishedRepo") {
		t.Fatal("wire.go must wire PublishedRepo from publishedRepo adapter")
	}
	if !strings.Contains(text, "ModelRepo:") || !strings.Contains(text, "draftRepo") {
		t.Fatal("wire.go must wire ModelRepo from draftRepo")
	}
	if strings.Contains(text, "PublishedRepo: dualStore") || strings.Contains(text, "PublishedRepo: dualStore,") {
		t.Fatal("PublishedRepo must not use dualStore; admin publish writes v2 snapshots only")
	}
}
