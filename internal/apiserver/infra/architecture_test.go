package infra_test

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

func TestEvaluationCommandRepositoriesDoNotExposeReadModelHelpers(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, rel := range []string{
		"internal/apiserver/infra/mysql/evaluation/assessment_repository.go",
		"internal/apiserver/infra/mysql/evaluation/score_repository.go",
		"internal/apiserver/infra/mongo/evaluation/repo.go",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			if isEvaluationReadModelHelper(fn.Name.Name) {
				t.Fatalf("%s defines %s; evaluation command repositories must keep list/count/trend reads in read-model adapters", rel, fn.Name.Name)
			}
			if strings.Contains(fn.Name.Name, "SaveWith") || fn.Name.Name == "SaveScores" {
				t.Fatalf("%s defines %s; evaluation command repositories must not expose deprecated outbox fallback writes", rel, fn.Name.Name)
			}
		}
	}
}

func isEvaluationReadModelHelper(name string) bool {
	for _, token := range []string{
		"List",
		"Count",
		"FindByTestee",
		"FindByOrg",
		"FindByIDs",
		"FindPending",
		"FindByPlan",
		"FindHighRisk",
		"FindLatest",
		"FindByAssessmentID",
		"FindBySpec",
	} {
		if strings.Contains(name, token) {
			return true
		}
	}
	return false
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
