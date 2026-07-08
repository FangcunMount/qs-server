package modelcatalog_test

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

func TestModelCatalogRootPackageOnlyFacadeFiles(t *testing.T) {
	t.Parallel()

	root := modelCatalogRoot(t)
	allowed := map[string]struct{}{
		"doc.go":    {},
		"errors.go": {},
		"export.go": {},
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if _, ok := allowed[name]; !ok {
			t.Fatalf("unexpected root file %s; modelcatalog root must only contain doc.go, errors.go, export.go", name)
		}
	}
}

func TestModelCatalogExportOnlyImportsSubpackages(t *testing.T) {
	t.Parallel()

	root := modelCatalogRoot(t)
	exportPath := filepath.Join(root, "export.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), exportPath, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	const modulePrefix = "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/"
	for _, imp := range parsed.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if !strings.HasPrefix(path, modulePrefix) {
			t.Fatalf("export.go imports %q; root facade may only import modelcatalog subpackages", path)
		}
		sub := strings.TrimPrefix(path, modulePrefix)
		if sub == "" || strings.Contains(sub, "/") && !isAllowedExportSubpackageRoot(sub) {
			t.Fatalf("export.go imports %q; allowed subpackages are factor, scoring, norming, typology, taskperformance, binding, publishing, legacy", path)
		}
	}
}

func TestModelCatalogExportHasNoNonAliasBusinessLogic(t *testing.T) {
	t.Parallel()

	root := modelCatalogRoot(t)
	exportPath := filepath.Join(root, "export.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), exportPath, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		t.Fatalf("export.go defines %s(); root facade must not contain business logic", fn.Name.Name)
	}
}

func isAllowedExportSubpackageRoot(sub string) bool {
	if idx := strings.Index(sub, "/"); idx >= 0 {
		sub = sub[:idx]
	}
	switch sub {
	case "factor", "scoring", "norming", "typology", "taskperformance",
		"binding", "publishing", "legacy":
		return true
	default:
		return false
	}
}

func TestModelCatalogTopLevelPackages(t *testing.T) {
	t.Parallel()

	root := modelCatalogRoot(t)
	required := []string{
		"factor", "scoring", "norming", "typology", "taskperformance",
		"binding", "publishing", "legacy",
	}
	transitional := map[string]struct{}{
		"personality": {},
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		seen[name] = struct{}{}
		if _, ok := transitional[name]; ok {
			continue
		}
		allowed := false
		for _, req := range required {
			if name == req {
				allowed = true
				break
			}
		}
		if !allowed {
			t.Fatalf("unexpected top-level package %q; canonical homes are %v (transitional compat seams only)", name, required)
		}
	}
	for _, req := range required {
		if _, ok := seen[req]; !ok {
			t.Fatalf("missing required top-level package %q", req)
		}
	}
}

func modelCatalogRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}
