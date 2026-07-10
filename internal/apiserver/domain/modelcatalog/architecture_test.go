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
			t.Fatalf("export.go imports %q; allowed subpackages are target model packages plus transitional mechanism packages", path)
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
	case "identity", "assessmentmodel", "definition", "factor", "norm", "conclusion", "payloadformat",
		"norming", "taskperformance", "binding":
		return true
	default:
		return false
	}
}

func TestModelCatalogTopLevelPackages(t *testing.T) {
	t.Parallel()

	root := modelCatalogRoot(t)
	required := []string{
		"identity", "assessmentmodel", "definition", "factor", "norm", "conclusion", "payloadformat",
		"norming", "taskperformance", "binding",
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

func TestTargetDomainPackagesDoNotDependOnPublishing(t *testing.T) {
	t.Parallel()

	root := modelCatalogRoot(t)
	targetPackages := []string{
		"assessmentmodel",
		"binding",
		"conclusion",
		"definition",
		"factor",
		"identity",
		"norm",
		"payloadformat",
	}
	for _, pkg := range targetPackages {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			t.Parallel()

			pkgRoot := filepath.Join(root, pkg)
			err := filepath.WalkDir(pkgRoot, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return nil
				}
				parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
				if err != nil {
					return err
				}
				for _, imp := range parsed.Imports {
					importPath := strings.Trim(imp.Path.Value, `"`)
					if strings.Contains(importPath, "/domain/modelcatalog/publishing") {
						t.Fatalf("%s must not import publishing compatibility package", path)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestModelCatalogDomainDoesNotImportRuntimePayloadAdapters(t *testing.T) {
	t.Parallel()

	root := modelCatalogRoot(t)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range parsed.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(importPath, "/port/modelcatalog/payload") {
				t.Fatalf("%s imports %s; runtime JSON adapters must not be dependencies of modelcatalog domain packages", path, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProductionCodeDoesNotDependOnPublishingFacade(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(filepath.Join(modelCatalogRoot(t), "..", "..", "..", ".."))
	targets := []string{
		filepath.Join(repoRoot, "internal", "apiserver", "application"),
		filepath.Join(repoRoot, "internal", "apiserver", "infra"),
		filepath.Join(repoRoot, "internal", "apiserver", "transport"),
		filepath.Join(repoRoot, "internal", "collection-server"),
	}
	for _, target := range targets {
		target := target
		t.Run(filepath.ToSlash(strings.TrimPrefix(target, repoRoot+string(filepath.Separator))), func(t *testing.T) {
			t.Parallel()

			err := filepath.WalkDir(target, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return nil
				}
				parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
				if err != nil {
					return err
				}
				for _, imp := range parsed.Imports {
					importPath := strings.Trim(imp.Path.Value, `"`)
					if strings.Contains(importPath, "/domain/modelcatalog/publishing") {
						t.Fatalf("%s must not import publishing compatibility package", path)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestModelCatalogExportReExportsMechanismRoots(t *testing.T) {
	t.Parallel()

	root := modelCatalogRoot(t)
	exportPath := filepath.Join(root, "export.go")
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	required := []string{
		"Product",
		"Identity",
		"Definition",
		"Conclusion",
	}
	for _, symbol := range required {
		if !strings.Contains(text, symbol) {
			t.Fatalf("export.go missing mechanism re-export %q", symbol)
		}
	}
	forbidden := []string{
		"Published" + "ModelSnapshot",
		"Model" + "Definition",
		"Decision" + "Spec",
		"Source" + "Ref",
		"Build" + "PublishedSnapshot",
		"Build" + "ScoringPublishedSnapshotFromScale",
	}
	for _, symbol := range forbidden {
		if strings.Contains(text, symbol) {
			t.Fatalf("export.go should not re-export runtime published DTO/builder %q", symbol)
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
