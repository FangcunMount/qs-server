package cachetarget

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const modulePrefix = "github.com/FangcunMount/qs-server/"

func TestApplicationCacheGovernanceBoundary(t *testing.T) {
	root := findRepoRoot(t)
	matched := scanProductionImports(t, filepath.Join(root, "internal", "apiserver", "application"), func(file, imported string) {
		if imported == modulePrefix+"internal/apiserver/cache/governance/model" ||
			imported == modulePrefix+"internal/apiserver/cache/governance/target" ||
			imported == modulePrefix+"internal/pkg/cache" {
			return
		}
		if strings.HasPrefix(imported, modulePrefix+"internal/pkg/redisruntime/observability") && strings.Contains(filepath.ToSlash(file), "/application/systemgovernance/") {
			return
		}
		for _, forbidden := range []string{
			modulePrefix + "internal/apiserver/cache/governance",
			modulePrefix + "internal/apiserver/cache/survey",
			modulePrefix + "internal/apiserver/cache/modelcatalog",
			modulePrefix + "internal/apiserver/cache/evaluation",
			modulePrefix + "internal/apiserver/cache/actor",
			modulePrefix + "internal/apiserver/cache/plan",
			modulePrefix + "internal/apiserver/cache/statistics",
			modulePrefix + "internal/pkg/redisruntime",
			"github.com/redis/go-redis",
		} {
			if imported == forbidden || strings.HasPrefix(imported, forbidden+"/") {
				t.Fatalf("%s imports forbidden cache implementation %s", rel(t, root, file), imported)
			}
		}
	})
	if matched == 0 {
		t.Fatal("application cache boundary scan matched zero production files")
	}
}

func TestGovernanceContractsStayInfrastructureFree(t *testing.T) {
	root := findRepoRoot(t)
	matched := 0
	for _, dir := range []string{
		filepath.Join(root, "internal", "apiserver", "cache", "governance", "model"),
		filepath.Join(root, "internal", "apiserver", "cache", "governance", "target"),
	} {
		matched += scanProductionImports(t, dir, func(file, imported string) {
			for _, forbidden := range []string{
				modulePrefix + "internal/pkg/redisruntime",
				"github.com/redis/go-redis",
				modulePrefix + "internal/apiserver/domain",
				modulePrefix + "internal/apiserver/port",
				modulePrefix + "internal/apiserver/infra",
			} {
				if imported == forbidden || strings.HasPrefix(imported, forbidden+"/") {
					t.Fatalf("%s imports forbidden infrastructure/business package %s", rel(t, root, file), imported)
				}
			}
		})
	}
	if matched == 0 {
		t.Fatal("governance contract scan matched zero production files")
	}
}

func scanProductionImports(t *testing.T, root string, visit func(file, imported string)) int {
	t.Helper()
	matched := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		matched++
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range parsed.Imports {
			visit(path, strings.Trim(spec.Path.Value, `"`))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan %s: %v", root, err)
	}
	return matched
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("repository root not found")
		}
		wd = parent
	}
}

func rel(t *testing.T, root, file string) string {
	t.Helper()
	value, err := filepath.Rel(root, file)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(value)
}
