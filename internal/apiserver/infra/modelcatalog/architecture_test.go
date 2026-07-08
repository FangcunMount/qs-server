package modelcatalog_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeReadPathsDoNotImportMongoRuleSetRepository(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	modulePrefix := "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset"
	allowlist := map[string]struct{}{
		"internal/apiserver/infra/mongo/ruleset":                  {},
		"internal/apiserver/infra/mongo/modelcatalog/backfill.go": {},
		"internal/apiserver/infra/mongo/modelcatalog":             {},
	}

	err := filepath.WalkDir(filepath.Join(root, "internal", "apiserver"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		if strings.HasPrefix(rel, "internal/apiserver/infra/mongo/ruleset/") {
			return nil
		}
		if _, ok := allowlist[rel]; ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), modulePrefix) {
			t.Fatalf("%s imports %s; runtime reads must use published_assessment_models only", rel, modulePrefix)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
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
