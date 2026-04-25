package securityplane_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecurityPlaneModelsStayTransportAndSDKFree(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	scanGoFiles(t, filepath.Join(root, "internal/pkg/securityplane"), func(path string, file *ast.File) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			forbiddenPrefixes := []string{
				"github.com/gin-gonic/gin",
				"google.golang.org/grpc",
				"github.com/FangcunMount/iam-contracts/",
				"github.com/FangcunMount/qs-server/internal/apiserver/",
			}
			for _, prefix := range forbiddenPrefixes {
				if strings.HasPrefix(importPath, prefix) {
					t.Fatalf("%s imports %s; securityplane must stay read-only and transport agnostic", path, importPath)
				}
			}
		}
	})
}

func TestBusinessHandlersDoNotAuthorizeWithJWTRoles(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	paths := []string{
		"internal/apiserver/interface/restful/handler",
		"internal/apiserver/transport/rest",
		"internal/collection-server/transport/rest/handler",
		"internal/worker/handlers",
	}
	for _, rel := range paths {
		scanGoSourceFiles(t, filepath.Join(root, rel), func(path string, content string) {
			forbidden := []string{
				"RequireRoleMiddleware(",
				"RequireAnyRoleMiddleware(",
				"RequireRole(",
				"RequireAnyRole(",
				"GetRoles(",
			}
			for _, token := range forbidden {
				if strings.Contains(content, token) {
					t.Fatalf("%s uses %s; business authorization must go through authz snapshot/capability decisions", path, token)
				}
			}
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
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
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		visit(path, file)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func scanGoSourceFiles(t *testing.T, root string, visit func(path string, content string)) {
	t.Helper()
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		bytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		visit(path, string(bytes))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
