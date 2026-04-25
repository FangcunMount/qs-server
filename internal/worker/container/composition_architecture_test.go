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

func TestWorkerContainerDoesNotExposePerClientSetters(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	scanGoFiles(t, filepath.Join(root, "internal", "worker", "container"), func(path string, file *ast.File) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !strings.HasPrefix(fn.Name.Name, "Set") {
				continue
			}
			t.Fatalf("%s exposes %s; worker runtime clients must be injected as a ClientBundle", path, fn.Name.Name)
		}
	})
}

func TestWorkerMessagingDoesNotImportContainerOrHandlers(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	scanGoFiles(t, filepath.Join(root, "internal", "worker", "integration", "messaging"), func(path string, file *ast.File) {
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if strings.HasPrefix(importPath, "github.com/FangcunMount/qs-server/internal/worker/container") ||
				strings.HasPrefix(importPath, "github.com/FangcunMount/qs-server/internal/worker/handlers") {
				t.Fatalf("%s imports %s; messaging runtime must depend on narrow interfaces", path, importPath)
			}
		}
	})
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
