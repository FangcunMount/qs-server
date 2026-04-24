package cache

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestObjectCachePackageDoesNotImportQueryHotsetOrTargetPackages(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current file")
	}
	cacheDir := filepath.Dir(currentFile)
	disallowed := map[string]struct{}{
		"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget":       {},
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachehotset": {},
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery":  {},
	}

	files, err := filepath.Glob(filepath.Join(cacheDir, "*.go"))
	if err != nil {
		t.Fatalf("glob cache package files: %v", err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports for %s: %v", file, err)
		}
		for _, imported := range parsed.Imports {
			path := strings.Trim(imported.Path.Value, `"`)
			if _, ok := disallowed[path]; ok {
				t.Fatalf("%s imports %s; infra/cache must stay object-cache only", file, path)
			}
		}
	}
}

func TestObjectCachePackageDoesNotReintroduceQueryFacadeFiles(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current file")
	}
	cacheDir := filepath.Dir(currentFile)
	for _, name := range []string{
		"local_hot_cache.go",
		"my_assessment_list_cache.go",
		"version_token_store.go",
		"versioned_query_cache.go",
	} {
		if _, err := os.Stat(filepath.Join(cacheDir, name)); err == nil {
			t.Fatalf("query facade file %s must live outside infra/cache", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", name, err)
		}
	}
}
