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

func TestModelCatalogScalePackageOnlyCompatSeams(t *testing.T) {
	t.Parallel()

	root := modelCatalogRoot(t)
	scaleRoot := filepath.Join(root, "scale")
	entries, err := os.ReadDir(scaleRoot)
	if err != nil {
		t.Fatal(err)
	}
	allowedSubdirs := map[string]struct{}{
		"definition": {},
		"snapshot":   {},
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Fatalf("unexpected top-level file %s in scale/; package must only contain compat seams", entry.Name())
		}
		name := entry.Name()
		if _, ok := allowedSubdirs[name]; !ok {
			t.Fatalf("unexpected scale subpackage %q; allowed compat seams are definition and snapshot", name)
		}
		subRoot := filepath.Join(scaleRoot, name)
		subEntries, err := os.ReadDir(subRoot)
		if err != nil {
			t.Fatal(err)
		}
		for _, subEntry := range subEntries {
			if subEntry.IsDir() {
				if name == "definition" && subEntry.Name() == "hotrank" {
					continue
				}
				t.Fatalf("unexpected nested directory scale/%s/%s; compat seams must stay flat", name, subEntry.Name())
			}
			subName := subEntry.Name()
			if !strings.HasSuffix(subName, ".go") || strings.HasSuffix(subName, "_test.go") {
				continue
			}
			if subName != "compat.go" {
				t.Fatalf("unexpected file scale/%s/%s; compat seams may only contain compat.go", name, subName)
			}
		}
	}
	assertScaleCompatReexportsScoring(t, scaleRoot)
}

func assertScaleCompatReexportsScoring(t *testing.T, scaleRoot string) {
	t.Helper()

	const scoringPrefix = "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/"
	err := filepath.WalkDir(scaleRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, "compat.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		if len(parsed.Imports) == 0 {
			t.Fatalf("%s must re-export from scoring canonical home", path)
		}
		for _, imp := range parsed.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if !strings.HasPrefix(importPath, scoringPrefix) {
				t.Fatalf("%s imports %q; scale compat seams may only re-export scoring packages", path, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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
		"cognitive":         {},
		"behavioral_rating": {},
		"scale":             {}, // compat-only: definition + snapshot seams until callers fully migrate
		"personality":       {},
		"identity":          {},
		"routing":           {},
		"catalog":           {},
		"capability":        {},
		"task_performance":  {},
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
