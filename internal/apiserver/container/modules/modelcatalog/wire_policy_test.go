package modelcatalog

import (
	"os"
	"strings"
	"testing"
)

func TestCatalogDepsUseDedicatedPublishedWriteRepository(t *testing.T) {
	content, err := os.ReadFile("wire.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(content)
	for _, required := range []string{"func buildCatalogDeps", "PublishedRepo:", "publishedRepo", "ModelRepo:", "draftRepo"} {
		if !strings.Contains(text, required) {
			t.Fatalf("wire.go must contain %q", required)
		}
	}
	if strings.Contains(text, "application/modelcatalog/"+"typology") || strings.Contains(text, "Typology"+"Deps") {
		t.Fatal("wire.go must not assemble family command dependencies")
	}
}
