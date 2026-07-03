package contract_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWorkerHandlersDoNotImportAPIServerDomain(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbidden := "github.com/FangcunMount/qs-server/internal/apiserver/domain/"
	scanImports(t, filepath.Join(root, "internal", "worker", "handlers"), func(path, importPath string) {
		if strings.HasPrefix(importPath, forbidden) {
			rel := filepath.ToSlash(mustRel(t, root, path))
			t.Fatalf("%s imports %s; worker handlers must use internal/pkg/eventpayload or eventoutcome", rel, importPath)
		}
	})
}

func TestCrossServiceConsumersImportSharedGRPCContract(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbidden := "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/"
	allowedPrefix := "github.com/FangcunMount/qs-server/api/grpc/gen/"

	scanRoots := []string{
		filepath.Join(root, "internal", "collection-server"),
		filepath.Join(root, "internal", "worker"),
	}
	for _, scanRoot := range scanRoots {
		scanImports(t, scanRoot, func(path, importPath string) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}
			if strings.Contains(importPath, forbidden) {
				rel := filepath.ToSlash(mustRel(t, root, path))
				t.Fatalf("%s imports deprecated proto path %s; use %s", rel, importPath, allowedPrefix)
			}
		})
	}
}

func TestWorkerOutcomeHandlersUseSharedEventOutcomeContract(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	want := "github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
	files := []string{
		filepath.Join(root, "internal", "worker", "handlers", "assessment_handler.go"),
		filepath.Join(root, "internal", "worker", "handlers", "report_handler.go"),
		filepath.Join(root, "internal", "worker", "handlers", "risk_attention.go"),
	}
	for _, path := range files {
		imports := fileImports(t, path)
		if !imports[want] {
			rel := filepath.ToSlash(mustRel(t, root, path))
			t.Fatalf("%s must import %s for outcome-enriched event payloads", rel, want)
		}
	}
}

func scanImports(t *testing.T, root string, visit func(path, importPath string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		for importPath := range fileImports(t, path) {
			visit(path, importPath)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func fileImports(t *testing.T, path string) map[string]bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	imports := make(map[string]bool, len(parsed.Imports))
	for _, imported := range parsed.Imports {
		imports[strings.Trim(imported.Path.Value, `"`)] = true
	}
	return imports
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
		t.Fatalf("rel path: %v", err)
	}
	return rel
}
