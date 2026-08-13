package container

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCollectionContainerDoesNotExposePerClientSetters(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	scanGoFiles(t, filepath.Join(root, "internal", "collection-server", "container"), func(path string, file *ast.File) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !strings.HasPrefix(fn.Name.Name, "Set") {
				continue
			}
			t.Fatalf("%s exposes %s; collection runtime clients must be injected as a ClientBundle", path, fn.Name.Name)
		}
	})
}

func TestCollectionContainerDoesNotImportApiserverRuntime(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, rel := range []string{
		"internal/collection-server/container",
		"internal/collection-server/transport",
	} {
		scanGoFiles(t, filepath.Join(root, rel), func(path string, file *ast.File) {
			for _, imported := range file.Imports {
				importPath := strings.Trim(imported.Path.Value, `"`)
				if strings.HasPrefix(importPath, "github.com/FangcunMount/qs-server/internal/apiserver/container") ||
					strings.HasPrefix(importPath, "github.com/FangcunMount/qs-server/internal/apiserver/transport") ||
					strings.HasPrefix(importPath, "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware") {
					t.Fatalf("%s imports %s; collection composition must not depend on apiserver runtime wiring", path, importPath)
				}
			}
		})
	}
}

func TestCatalogRuntimeSharesPublishedModelQueryServiceWithTypologyProjector(t *testing.T) {
	t.Parallel()

	path := filepath.Join(repoRoot(t), "internal", "collection-server", "container", "application_runtime.go")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{
		"assessmentModels := newAssessmentModelQueryService(",
		"assessmentModels: assessmentModels",
		"grpcbridge.NewTypologyCatalogProjector(assessmentModels)",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("catalog runtime must share one published-model QueryService; missing %q", required)
		}
	}
	if strings.Count(text, "newAssessmentModelQueryService(c.assessmentModelCatalogClient") != 1 {
		t.Fatal("catalog runtime constructed more than one published-model QueryService")
	}
}

func TestEvaluationRoutesShareOneCachedQueryService(t *testing.T) {
	t.Parallel()
	path := filepath.Join(repoRoot(t), "internal", "collection-server", "container", "container.go")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Count(text, "grpcbridge.NewEvaluationBFFReader(") != 1 {
		t.Fatal("collection container must construct exactly one evaluation BFF reader")
	}
	for _, required := range []string{
		"evaluation.WithAssessmentAccessCache(assessmentAccessCache, assessmentAccessSingleflight)",
		"evaluation.WithAssessmentDetailCache(assessmentDetailCache, assessmentDetailSingleflight)",
		"typologyassessment.NewQueryService(\n\t\tc.evaluationQueryService",
		"behaviorassessment.NewQueryService(\n\t\tc.evaluationQueryService",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("evaluation runtime does not share cached QueryService; missing %q", required)
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

func scanGoFiles(t *testing.T, root string, visit func(path string, file *ast.File)) {
	t.Helper()
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		visit(path, file)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
