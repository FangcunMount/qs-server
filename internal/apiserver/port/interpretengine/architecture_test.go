package interpretengine_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInterpretEnginePortOwnsExecutionDTOs(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "port", "interpretengine", "interpretengine.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imported := range parsed.Imports {
		importPath := strings.Trim(imported.Path.Value, `"`)
		if strings.Contains(importPath, "/domain/evaluation/interpretation") {
			t.Fatalf("port/interpretengine imports %s; execution DTOs must be port-owned", importPath)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}
	dir := filepath.Dir(file)
	for !strings.HasSuffix(dir, string(filepath.Separator)+"internal") {
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
	return filepath.Dir(dir)
}
