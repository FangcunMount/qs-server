package mongo_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSurveyScaleMongoCommandReposDoNotExposeReadModelMethods(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	files := []string{
		"internal/apiserver/infra/mongo/questionnaire/repo.go",
		"internal/apiserver/infra/mongo/answersheet/repo.go",
		"internal/apiserver/infra/mongo/scale/repo.go",
	}
	forbidden := map[string]struct{}{
		"FindBaseList":                   {},
		"FindBasePublishedList":          {},
		"CountWithConditions":            {},
		"CountPublishedWithConditions":   {},
		"FindSummaryList":                {},
		"FindSummaryListByFiller":        {},
		"FindSummaryListByQuestionnaire": {},
		"CountByFiller":                  {},
		"CountByQuestionnaire":           {},
	}

	for _, rel := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}
			if !isRepositoryReceiver(fn.Recv.List[0].Type) {
				continue
			}
			if _, ok := forbidden[fn.Name.Name]; ok {
				t.Fatalf("%s exposes Repository.%s; list/count queries belong to read-model adapters", rel, fn.Name.Name)
			}
		}
	}
}

func isRepositoryReceiver(expr ast.Expr) bool {
	switch receiver := expr.(type) {
	case *ast.StarExpr:
		return isRepositoryReceiver(receiver.X)
	case *ast.Ident:
		return receiver.Name == "Repository"
	default:
		return false
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
