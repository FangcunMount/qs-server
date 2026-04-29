package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var deprecatedImplementationPackages = []string{
	"internal/pkg/redisbootstrap",
	"internal/pkg/redisplane",
	"internal/pkg/rediskey",
	"internal/pkg/redislock",
	"internal/pkg/cacheobservability",
	"internal/pkg/processruntime",
}

func TestDeprecatedImplementationPackagesHaveNoLocalGoFiles(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range deprecatedImplementationPackages {
		entries, err := os.ReadDir(filepath.Join(root, rel))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" {
				t.Fatalf("%s/%s remains after extraction; use the domain package or component-base adapter", rel, entry.Name())
			}
		}
	}
}

func TestProductionGoDoesNotImportDeprecatedImplementationPackages(t *testing.T) {
	root := repoRoot(t)
	modulePrefix := "github.com/FangcunMount/qs-server/"
	forbiddenImports := make([]string, 0, len(deprecatedImplementationPackages))
	for _, rel := range deprecatedImplementationPackages {
		forbiddenImports = append(forbiddenImports, modulePrefix+rel)
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "docs", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, forbidden := range forbiddenImports {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s imports deprecated implementation package %s", mustRel(t, root, path), forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk production Go files: %v", err)
	}
}

func TestCurrentDocsDoNotReferenceDeletedImplementationPaths(t *testing.T) {
	root := repoRoot(t)
	docsRoot := filepath.Join(root, "docs")
	forbiddenTerms := []string{
		"internal/pkg/redisbootstrap",
		"internal/pkg/redisplane",
		"internal/pkg/rediskey",
		"internal/pkg/redislock",
		"internal/pkg/cacheobservability",
		"internal/pkg/processruntime",
		"redisbootstrap",
		"redisplane",
		"rediskey",
		"redislock",
		"cacheobservability",
	}

	err := filepath.WalkDir(docsRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if filepath.ToSlash(mustRel(t, root, path)) == "docs/_archive" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, forbidden := range forbiddenTerms {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s references deleted implementation path or package %q", mustRel(t, root, path), forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs: %v", err)
	}
}
