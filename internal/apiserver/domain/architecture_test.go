package domain_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDomainDoesNotImportInfrastructureDrivers(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	domainRoot := filepath.Join(root, "internal", "apiserver", "domain")
	forbiddenImports := []string{
		"github.com/FangcunMount/qs-server/internal/apiserver/infra",
		"github.com/redis/",
		"github.com/go-redis/",
		"go.mongodb.org/mongo-driver",
		"gorm.io/",
	}

	err := filepath.WalkDir(domainRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			for _, forbidden := range forbiddenImports {
				if strings.HasPrefix(importPath, forbidden) {
					rel := filepath.ToSlash(mustRel(t, root, path))
					t.Fatalf("%s imports %s; domain packages must stay free of infrastructure drivers", rel, importPath)
				}
			}
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
