package ruleengine_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRuleEnginePortDoesNotDependOnCalculationExecutionModel(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "port", "ruleengine", "ruleengine.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imported := range parsed.Imports {
		importPath := strings.Trim(imported.Path.Value, `"`)
		if importPath == "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation" {
			t.Fatalf("port/ruleengine imports %s; execution value interfaces must be port-owned", importPath)
		}
	}
}

func TestEvaluationPipelineDoesNotCallCalculationExecutionDirectly(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	pipelineRoot := filepath.Join(root, "internal", "apiserver", "application", "evaluation", "engine", "pipeline")
	err := filepath.WalkDir(pipelineRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, token := range []string{
			"internal/apiserver/domain/calculation",
			"calculation.GetStrategy",
			"calculation.BatchScore",
		} {
			if strings.Contains(text, token) {
				t.Fatalf("%s contains %q; evaluation pipeline must use ruleengine ports for scoring execution", filepath.ToSlash(mustRel(t, root, path)), token)
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
