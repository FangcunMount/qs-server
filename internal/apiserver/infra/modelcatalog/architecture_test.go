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
		"internal/apiserver/infra/mongo/ruleset":      {},
		"internal/apiserver/infra/mongo/modelcatalog": {},
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
			t.Fatalf("%s imports %s; runtime reads must use modelcatalog published snapshots only", rel, modulePrefix)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProductionCodeDoesNotReferenceLayeredCatalogFallback(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbidden := []string{
		"NewLayeredCatalog",
		"LayeredCatalog",
		"CatalogStoreLayeredStatic",
		"qs_modelcatalog_legacy_fallback_hits_total",
		"RecordLegacyFallback",
		"ruleset.NewCatalog",
		"rulesetInfra.NewCatalog",
	}
	allowPrefixes := []string{
		"scripts/",
		"internal/apiserver/infra/ruleset/static_composite_catalog",
		"internal/apiserver/infra/ruleset/factory.go",
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
		for _, prefix := range allowPrefixes {
			if strings.HasPrefix(rel, prefix) {
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Fatalf("%s must not reference retired fallback token %s", rel, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProductionCodeDoesNotReferenceDefaultStaticCatalog(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	token := "NewDefaultStaticCatalog"
	allowPrefixes := []string{
		"scripts/",
		"internal/apiserver/infra/ruleset/",
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" || entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		if strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		for _, prefix := range allowPrefixes {
			if strings.HasPrefix(rel, prefix) {
				return nil
			}
		}
		if strings.HasPrefix(rel, "internal/apiserver/container/") {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), token) {
				t.Fatalf("%s must not reference %s; production uses NewRuntimePublishedCatalog only", rel, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRuntimePathsDoNotCallLegacyPublishedReaderMethods(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbidden := []string{
		".GetPublishedByRef(",
		".FindPublishedByQuestionnaire(",
	}
	scanRoots := []string{
		"internal/apiserver/infra/ruleset",
		"internal/apiserver/infra/evaluationinput",
		"internal/apiserver/transport/grpc/service",
	}
	allowPrefixes := []string{
		"internal/apiserver/infra/ruleset/static_composite_catalog",
		"internal/apiserver/infra/ruleset/runtime_catalog_test.go",
		"internal/apiserver/infra/ruleset/runtime_catalog_v2_test.go",
	}

	for _, scanRoot := range scanRoots {
		scanRoot := scanRoot
		t.Run(scanRoot, func(t *testing.T) {
			t.Parallel()
			err := filepath.WalkDir(filepath.Join(root, scanRoot), func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return nil
				}
				rel := filepath.ToSlash(mustRel(t, root, path))
				for _, prefix := range allowPrefixes {
					if strings.HasPrefix(rel, prefix) {
						return nil
					}
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				text := string(data)
				for _, token := range forbidden {
					if strings.Contains(text, token) {
						t.Fatalf("%s must not call legacy v1 reader %s; use PublishedModelReader", rel, token)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
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
