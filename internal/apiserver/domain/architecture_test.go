package domain_test

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

func TestSurveyScaleDomainDoesNotDependOnOuterLayers(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	scanRoots := []string{
		filepath.Join(root, "internal", "apiserver", "domain", "survey"),
		filepath.Join(root, "internal", "apiserver", "domain", "scale"),
		filepath.Join(root, "internal", "apiserver", "domain", "calculation"),
		filepath.Join(root, "internal", "apiserver", "domain", "validation"),
	}
	forbiddenImports := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/application/": "application",
		"github.com/FangcunMount/qs-server/internal/apiserver/" + "infra/":  "infrastructure",
		"github.com/FangcunMount/qs-server/internal/apiserver/transport/":   "transport",
		"github.com/FangcunMount/qs-server/internal/apiserver/port/":        "port",
		"github.com/FangcunMount/component-base/pkg/logger":                 "technical logging",
		"github.com/FangcunMount/component-base/pkg/errors":                 "API error wrappers",
		"github.com/FangcunMount/qs-server/internal/pkg/code":               "API error codes",
	}

	for _, scanRoot := range scanRoots {
		scanGoImports(t, scanRoot, func(path, importPath string) {
			for forbidden, label := range forbiddenImports {
				if strings.HasPrefix(importPath, forbidden) {
					rel := filepath.ToSlash(mustRel(t, root, path))
					t.Fatalf("%s imports %s; survey/scale domain must not depend on %s", rel, importPath, label)
				}
			}
		})
	}
}

func TestScaleDomainDoesNotExposePersistencePayloadMappers(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	dir := filepath.Join(root, "internal", "apiserver", "domain", "scale")
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
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
			"map[string]interface{}",
			"ToMap(",
		} {
			if strings.Contains(text, token) {
				t.Fatalf("%s contains %q; persistence payload mapping belongs to infra mappers", filepath.ToSlash(mustRel(t, root, path)), token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSurveyScaleDomainRepositoriesStayCommandSide(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, rel := range []string{
		"internal/apiserver/domain/survey/questionnaire/repository.go",
		"internal/apiserver/domain/survey/answersheet/repository.go",
		"internal/apiserver/domain/scale/repository.go",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Name.Name != "Repository" {
					continue
				}
				iface, ok := typeSpec.Type.(*ast.InterfaceType)
				if !ok {
					continue
				}
				for _, method := range iface.Methods.List {
					if len(method.Names) == 0 {
						continue
					}
					name := method.Names[0].Name
					if strings.Contains(name, "List") || strings.Contains(name, "Count") {
						t.Fatalf("%s Repository.%s is a read-model method; domain repositories must stay command-side", rel, name)
					}
					if fieldListContainsMapStringInterface(method.Type) {
						t.Fatalf("%s Repository.%s uses map[string]interface{}; typed read filters belong to read-model ports", rel, name)
					}
				}
			}
		}
	}
}

func TestEvaluationDomainRepositoriesStayCommandSide(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, target := range []struct {
		rel        string
		interfaces []string
	}{
		{
			rel:        "internal/apiserver/domain/evaluation/assessment/repository.go",
			interfaces: []string{"Repository", "ScoreRepository"},
		},
		{
			rel:        "internal/apiserver/domain/evaluation/report/repository.go",
			interfaces: []string{"ReportRepository"},
		},
	} {
		path := filepath.Join(root, filepath.FromSlash(target.rel))
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !stringIn(typeSpec.Name.Name, target.interfaces) {
					continue
				}
				iface, ok := typeSpec.Type.(*ast.InterfaceType)
				if !ok {
					continue
				}
				for _, method := range iface.Methods.List {
					if len(method.Names) == 0 {
						continue
					}
					name := method.Names[0].Name
					if isEvaluationReadModelMethod(name) {
						t.Fatalf("%s %s.%s is a read-model method; evaluation domain repositories must stay command-side", target.rel, typeSpec.Name.Name, name)
					}
					if strings.Contains(name, "SaveWith") || name == "SaveScores" {
						t.Fatalf("%s %s.%s is a deprecated persistence fallback; use application UoW/outbox ports instead", target.rel, typeSpec.Name.Name, name)
					}
					if fieldListContainsMapStringInterface(method.Type) {
						t.Fatalf("%s %s.%s uses map[string]interface{}; typed read filters belong to read-model ports", target.rel, typeSpec.Name.Name, name)
					}
				}
			}
		}
	}
}

func TestEvaluationDomainDoesNotDependOnSurveyScaleOrOuterLayers(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	scanRoot := filepath.Join(root, "internal", "apiserver", "domain", "evaluation")
	forbiddenImports := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/application/":   "application",
		"github.com/FangcunMount/qs-server/internal/apiserver/" + "infra/":    "infrastructure",
		"github.com/FangcunMount/qs-server/internal/apiserver/transport/":     "transport",
		"github.com/FangcunMount/qs-server/internal/apiserver/port/":          "port",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale":   "scale domain",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/": "survey domain",
		"github.com/FangcunMount/component-base/pkg/logger":                   "technical logging",
		"github.com/FangcunMount/component-base/pkg/errors":                   "API error wrappers",
		"github.com/FangcunMount/qs-server/internal/pkg/code":                 "API error codes",
	}
	scanGoImports(t, scanRoot, func(path, importPath string) {
		for forbidden, label := range forbiddenImports {
			if strings.HasPrefix(importPath, forbidden) {
				rel := filepath.ToSlash(mustRel(t, root, path))
				t.Fatalf("%s imports %s; evaluation domain must not depend on %s", rel, importPath, label)
			}
		}
	})
}

func TestCalculationAndValidationDomainStayRuleOnly(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenTokens := []string{
		"RegisterStrategy",
		"GetStrategy(",
		"ScoringStrategy",
		"ValidationStrategy",
		"OptionScorer",
		"DefaultScorer",
		"DefaultValidator",
		"NewBatchScorer",
		"NewBatchValidator",
		"BatchScore",
		"BatchValidate",
		"defaultBatch",
	}
	for _, rel := range []string{
		"internal/apiserver/domain/calculation",
		"internal/apiserver/domain/validation",
	} {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
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
			for _, token := range forbiddenTokens {
				if strings.Contains(text, token) {
					t.Fatalf("%s contains %q; calculation/validation domain must only expose rule language and value objects", filepath.ToSlash(mustRel(t, root, path)), token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestEvaluationInterpretationDomainStayRuleOnly(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	dir := filepath.Join(root, "internal", "apiserver", "domain", "evaluation", "interpretation")
	forbiddenTokens := []string{
		"RegisterStrategy",
		"GetStrategy(",
		"GetDefaultInterpreter",
		"GetDefaultProvider",
		"DefaultInterpreter",
		"DefaultInterpretationProvider",
		"BatchInterpreter",
		"defaultInterpreter",
		"defaultProvider",
		"defaultBatch",
	}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
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
		for _, token := range forbiddenTokens {
			if strings.Contains(text, token) {
				t.Fatalf("%s contains %q; evaluation interpretation domain must only expose rule language and value objects", filepath.ToSlash(mustRel(t, root, path)), token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func stringIn(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func isEvaluationReadModelMethod(name string) bool {
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
	} {
		if strings.Contains(name, token) {
			return true
		}
	}
	return false
}

func fieldListContainsMapStringInterface(expr ast.Expr) bool {
	fn, ok := expr.(*ast.FuncType)
	if !ok {
		return false
	}
	return astFieldListContainsMapStringInterface(fn.Params) || astFieldListContainsMapStringInterface(fn.Results)
}

func astFieldListContainsMapStringInterface(fields *ast.FieldList) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		mapType, ok := field.Type.(*ast.MapType)
		if !ok {
			continue
		}
		key, keyOK := mapType.Key.(*ast.Ident)
		if !keyOK || key.Name != "string" {
			continue
		}
		if iface, ok := mapType.Value.(*ast.InterfaceType); ok && iface.Methods != nil && len(iface.Methods.List) == 0 {
			return true
		}
	}
	return false
}

func scanGoImports(t *testing.T, root string, visit func(path, importPath string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			visit(path, strings.Trim(imported.Path.Value, `"`))
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
