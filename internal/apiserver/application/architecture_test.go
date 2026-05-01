package application_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestApplicationsUsePortsForInfraBoundaries(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenImports := map[string]string{
		"go.mongodb.org/mongo-driver":                                 "repository ports, not Mongo driver errors",
		"github.com/FangcunMount/iam/":                                "IAM bridge ports, not generated IAM packages",
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/": "application ports, not infrastructure packages",
	}
	scanGoImports(t, filepath.Join(root, "internal", "apiserver", "application"), func(path, importPath string) {
		for forbidden, replacement := range forbiddenImports {
			if strings.HasPrefix(importPath, forbidden) {
				rel := filepath.ToSlash(mustRel(t, root, path))
				t.Fatalf("%s imports %s; application services must depend on %s", rel, importPath, replacement)
			}
		}
	})
}

func scanGoImports(t *testing.T, root string, visit func(path, importPath string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			visit(path, strings.Trim(imported.Path.Value, `"`))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func mustRel(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}
