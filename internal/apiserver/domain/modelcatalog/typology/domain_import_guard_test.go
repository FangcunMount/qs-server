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
			if path == "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog" {
				t.Fatalf("domain/modelcatalog/typology must not import modelcatalog root facade in %s", file)
			}
			if strings.Contains(path, "/internal/apiserver/application/") ||
				strings.Contains(path, "/internal/apiserver/infra/") ||
				strings.Contains(path, "/internal/apiserver/port/") {
				t.Fatalf("domain/modelcatalog/typology must not import %s in %s", path, file)
			}
		}
	}
}
