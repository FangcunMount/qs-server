package typology

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestTypologyDoesNotDependOnOuterLayers(t *testing.T) {
	t.Parallel()

	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("Glob returned error: %v", err)
	}
	for _, file := range matches {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("ParseFile(%s) returned error: %v", file, err)
		}
		for _, imp := range parsed.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(path, "/internal/apiserver/application/") ||
				strings.Contains(path, "/internal/apiserver/infra/") ||
				strings.Contains(path, "/internal/apiserver/port/") {
				t.Fatalf("domain/assessmentmodel/personality/typology must not import %s in %s", path, file)
			}
		}
	}
}
