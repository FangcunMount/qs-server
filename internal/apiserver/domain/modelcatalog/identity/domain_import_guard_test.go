package identity

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentityDoesNotDependOnBinding(t *testing.T) {
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
			if strings.Contains(strings.Trim(imp.Path.Value, `\"`), "/domain/modelcatalog/binding") {
				t.Fatalf("identity must not import binding compatibility package in %s", file)
			}
		}
	}
}
