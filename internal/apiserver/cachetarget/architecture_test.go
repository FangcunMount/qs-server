package cachetarget

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestApplicationHotsetPathsDoNotImportInfraCache(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current file")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	applicationRoot := filepath.Join(repoRoot, "internal", "apiserver", "application")
	for _, dir := range []string{
		filepath.Join(applicationRoot, "cachegovernance"),
		filepath.Join(applicationRoot, "statistics"),
		filepath.Join(applicationRoot, "scale"),
		filepath.Join(applicationRoot, "survey", "questionnaire"),
	} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir, err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") || strings.HasSuffix(file, filepath.Join("scale", "global_list_cache.go")) {
				continue
			}
			checkNoInfraCacheImport(t, file)
		}
	}
}

func checkNoInfraCacheImport(t *testing.T, file string) {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports for %s: %v", file, err)
	}
	for _, imported := range parsed.Imports {
		if strings.Trim(imported.Path.Value, `"`) == "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache" {
			t.Fatalf("%s imports infra/cache; hotset-facing application code should depend on cachetarget interfaces", file)
		}
	}
}
