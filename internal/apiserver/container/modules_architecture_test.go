package container

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
)

func TestMigratedModulePackagesHaveAssembleFile(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, pkg := range modules.MigratedModulePackages {
		for _, fileName := range modules.MigratedModuleAssembleFiles[pkg] {
			path := filepath.Join(root, "internal", "apiserver", "container", "modules", string(pkg), fileName)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("migrated module %s missing %s: %v", pkg, fileName, err)
			}
		}
	}
}

func TestMigratedModulePackagesHaveTransportExportFiles(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for pkg, files := range modules.MigratedModuleTransportExportFiles {
		for _, fileName := range files {
			path := filepath.Join(root, "internal", "apiserver", "container", "modules", string(pkg), fileName)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("module %s missing transport export file %s: %v", pkg, fileName, err)
			}
		}
	}
}

func TestModulePackageSkeletonExists(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, pkg := range modules.AllPackages {
		path := filepath.Join(root, "internal", "apiserver", "container", "modules", string(pkg), "module.go")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("module package %s missing module.go: %v", pkg, err)
		}
	}
}

func TestLegacyBootstrapFilesAreFrozen(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	containerRoot := filepath.Join(root, "internal", "apiserver", "container")
	allowed := make(map[string]struct{}, len(modules.LegacyBootstrapFiles))
	for _, name := range modules.LegacyBootstrapFiles {
		allowed[name] = struct{}{}
	}

	entries, err := os.ReadDir(containerRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "bootstrap_") || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		if _, ok := allowed[entry.Name()]; !ok {
			t.Fatalf("%s is a new flat bootstrap file; add business bootstrap under container/modules/ instead", entry.Name())
		}
	}
	for name := range allowed {
		if _, err := os.Stat(filepath.Join(containerRoot, name)); err != nil {
			t.Fatalf("allowlisted bootstrap file %s no longer exists; update modules.LegacyBootstrapFiles", name)
		}
	}
}

func TestMigratedModulePackagesHaveBootstrapFile(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for pkg, fileName := range modules.MigratedModuleBootstrapFiles {
		path := filepath.Join(root, "internal", "apiserver", "container", "modules", string(pkg), fileName)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("module %s missing bootstrap file %s: %v", pkg, fileName, err)
		}
	}
}

func TestPlatformBootstrapFilesExist(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, fileName := range modules.PlatformBootstrapFiles {
		path := filepath.Join(root, "internal", "apiserver", "container", "modules", "platform", fileName)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("platform module missing bootstrap file %s: %v", fileName, err)
		}
	}
}

func TestAssemblerPackageRemoved(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	assemblerRoot := filepath.Join(root, "internal", "apiserver", "container", "assembler")
	if _, err := os.Stat(assemblerRoot); err == nil {
		entries, readErr := os.ReadDir(assemblerRoot)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			if strings.HasSuffix(entry.Name(), ".go") {
				t.Fatalf("assembler/%s still exists; business assembly must live under container/modules/", entry.Name())
			}
		}
	}
}

func TestContainerInitializeSequenceMatchesRegistry(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "container", "root.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	var initFn *ast.FuncDecl
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Initialize" || fn.Recv == nil {
			continue
		}
		initFn = fn
		break
	}
	if initFn == nil || initFn.Body == nil {
		t.Fatal("Container.Initialize not found")
	}

	wantInitMethods := make([]string, 0, len(modules.LegacyInitializeSequence))
	allowed := make(map[string]struct{}, len(modules.LegacyInitializeSequence))
	for _, step := range modules.LegacyInitializeSequence {
		wantInitMethods = append(wantInitMethods, step.InitMethod)
		allowed[step.InitMethod] = struct{}{}
	}

	gotInitMethods := make([]string, 0, len(wantInitMethods))
	for _, stmt := range initFn.Body.List {
		gotInitMethods = append(gotInitMethods, initMethodsFromStmt(stmt, allowed)...)
	}
	if !reflect.DeepEqual(gotInitMethods, wantInitMethods) {
		t.Fatalf("Initialize init methods = %v, want %v; update modules.LegacyInitializeSequence when changing init order", gotInitMethods, wantInitMethods)
	}
}

func initMethodsFromStmt(stmt ast.Stmt, allowed map[string]struct{}) []string {
	switch typed := stmt.(type) {
	case *ast.ExprStmt:
		if method := initMethodFromCall(typed.X, allowed); method != "" {
			return []string{method}
		}
	case *ast.AssignStmt:
		for _, expr := range typed.Rhs {
			if method := initMethodFromCall(expr, allowed); method != "" {
				return []string{method}
			}
		}
	case *ast.IfStmt:
		if typed.Init != nil {
			if methods := initMethodsFromStmt(typed.Init, allowed); len(methods) > 0 {
				return methods
			}
		}
	}
	return nil
}

func initMethodFromCall(expr ast.Expr, allowed map[string]struct{}) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if _, ok := allowed[selector.Sel.Name]; !ok {
		return ""
	}
	return selector.Sel.Name
}

func TestRegisterModuleCallsMatchRegistry(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	containerRoot := filepath.Join(root, "internal", "apiserver", "container")
	got := make([]string, 0, len(modules.LegacyRegisteredModuleOrder()))

	for _, step := range modules.LegacyInitializeSequence {
		names := extractRegisterModuleNames(t, containerRoot, step)
		if !reflect.DeepEqual(names, step.RegisterNames) {
			t.Fatalf("%s registerModule names = %v, want %v", step.InitMethod, names, step.RegisterNames)
		}
		got = append(got, names...)
	}

	want := modules.LegacyRegisteredModuleOrder()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("registerModule order = %v, want %v; update modules.LegacyInitializeSequence when changing registration", got, want)
	}
}

func extractRegisterModuleNames(t *testing.T, containerRoot string, step modules.LegacyInitStep) []string {
	t.Helper()

	switch step.InitMethod {
	case "initModelCatalogModule":
		return extractRegisterModuleNamesFromFunc(t, filepath.Join(containerRoot, "compose_host.go"), "SetAssessmentModelModule")
	default:
		installRel, ok := installFileByInitMethod[step.InitMethod]
		if !ok {
			t.Fatalf("no install.go mapping for %s", step.InitMethod)
		}
		return extractRegisterModuleNamesFromFunc(t, filepath.Join(containerRoot, installRel), "InstallFrom")
	}
}

var installFileByInitMethod = map[string]string{
	"initSurveyModule":     "modules/survey/install.go",
	"initActorModule":      "modules/actor/install.go",
	"initReportModule":     "modules/interpretation/install.go",
	"initEvaluationModule": "modules/evaluation/install.go",
	"initPlanModule":       "modules/plan/install.go",
	"initStatisticsModule": "modules/statistics/install.go",
}

func extractRegisterModuleNamesFromFunc(t *testing.T, path, funcName string) []string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	var targetFn *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != funcName || fn.Body == nil {
			continue
		}
		targetFn = fn
		break
	}
	if targetFn == nil {
		t.Fatalf("%s not found in %s", funcName, path)
	}
	return collectRegisterModuleNames(targetFn.Body)
}

func collectRegisterModuleNames(body ast.Node) []string {
	names := make([]string, 0)
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if fun.Sel.Name != "registerModule" && fun.Sel.Name != "RegisterModule" {
				return true
			}
		case *ast.Ident:
			if fun.Name != "registerModule" && fun.Name != "RegisterModule" {
				return true
			}
		default:
			return true
		}
		if len(call.Args) < 1 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		names = append(names, strings.Trim(lit.Value, `"`))
		return true
	})
	return names
}

func TestAssessmentModelDoesNotRegisterLegacyModuleNames(t *testing.T) {
	t.Parallel()

	for _, step := range modules.LegacyInitializeSequence {
		if step.InitMethod != "initModelCatalogModule" {
			continue
		}
		for _, name := range step.RegisterNames {
			if name == "scale" || name == "typologymodel" {
				t.Fatalf("initModelCatalogModule must not register legacy name %q", name)
			}
		}
	}
}

func TestModelCatalogRegistersAggregateName(t *testing.T) {
	t.Parallel()

	found := false
	for _, step := range modules.LegacyInitializeSequence {
		if step.InitMethod != "initModelCatalogModule" {
			continue
		}
		for _, name := range step.RegisterNames {
			if name == string(modules.PackageModelCatalog) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("initModelCatalogModule must register modelcatalog aggregate module name")
	}
}

func TestEvaluationAndReportModulesUseAssessmentModelCatalogPort(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, rel := range []string{
		"internal/apiserver/container/modules/evaluation/install.go",
		"internal/apiserver/container/modules/interpretation/install.go",
	} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		if !strings.Contains(content, "DefaultEvaluationCatalog()") {
			t.Fatalf("%s must resolve descriptors through DefaultEvaluationCatalog", rel)
		}
		for _, token := range []string{
			"typologyEvaluation.DefaultModules()",
			"typologyEvaluation.DefaultModuleRegistry()",
			"typologyEvaluation.DefaultPersonalityRuntimeRegistry()",
			"typologyEvaluation.DefaultTypologyDescriptors()",
		} {
			if strings.Contains(content, token) {
				t.Fatalf("%s contains %s; downstream modules must depend on assessmentmodel catalog ports", rel, token)
			}
		}
	}
}

func TestEvaluationModuleDoesNotOwnDefaultTypologyCatalog(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, rel := range []string{
		"internal/apiserver/container/modules/evaluation/assemble.go",
		"internal/apiserver/container/modules/evaluation/descriptors.go",
	} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		for _, token := range []string{
			"DefaultModules()",
			"DefaultModuleRegistry(",
			"DefaultDescriptors(",
			"DefaultWiringDeps(",
		} {
			if strings.Contains(content, token) {
				t.Fatalf("%s contains %s; default typology catalog must be owned by assessmentmodel composition", rel, token)
			}
		}
	}
}

func TestContainerDoesNotImportFactorMechanismPackagesDirectly(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenImports := []string{
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/task_performance",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/task_performance",
	}
	scanRoot := filepath.Join(root, "internal", "apiserver", "container")
	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
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
		for _, imp := range forbiddenImports {
			if strings.Contains(text, imp) {
				t.Fatalf("%s imports %s; container wiring must use application/evaluation/registry", filepath.ToSlash(mustRel(t, root, path)), imp)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
