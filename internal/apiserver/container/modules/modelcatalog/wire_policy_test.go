package modelcatalog

import (
	"os"
	"strings"
	"testing"
)

func TestTypologyDepsUseSeparatePublishedWriteRepository(t *testing.T) {
	content, err := os.ReadFile("wire.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "PublishedRepo:") || !strings.Contains(text, "publishedRepo") {
		t.Fatal("wire.go must wire PublishedRepo from publishedRepo adapter")
	}
	if !strings.Contains(text, "PublishedReader:") || !strings.Contains(text, "publishedReader") {
		t.Fatal("wire.go must wire PublishedReader from published model store")
	}
	if !strings.Contains(text, "ModelRepo:") || !strings.Contains(text, "draftRepo") {
		t.Fatal("wire.go must wire ModelRepo from draftRepo")
	}
	if strings.Contains(text, "PublishedRepo: dualStore") || strings.Contains(text, "PublishedRepo: dualStore,") {
		t.Fatal("PublishedRepo must not use dualStore; admin publish writes v2 snapshots only")
	}
}

func TestBootstrapPassesAssessmentModelRepositoriesToScoring(t *testing.T) {
	content, err := os.ReadFile("bootstrap.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "scoringDeps.ModelRepo = in.Typology.ModelRepo") {
		t.Fatal("Bootstrap must pass draft assessment model repository to scoring publish bridge")
	}
	if !strings.Contains(text, "scoringDeps.PublishedRepo = in.Typology.PublishedRepo") {
		t.Fatal("Bootstrap must pass published assessment model repository to scoring publish bridge")
	}
	if !strings.Contains(text, "scoringDeps.PublishedReader = in.Typology.PublishedReader") {
		t.Fatal("Bootstrap must pass published assessment model reader to scoring query bridge")
	}
}
