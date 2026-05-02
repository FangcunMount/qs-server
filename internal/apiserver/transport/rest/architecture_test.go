package rest

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRESTTransportDoesNotDependOnLegacyRESTImplementation(t *testing.T) {
	t.Parallel()

	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "internal/apiserver/interface/restful") {
			t.Fatalf("transport/rest must own REST implementation and not import legacy interface/restful: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSurveyScaleRESTHandlersDoNotConstructDomainRules(t *testing.T) {
	t.Parallel()

	forbiddenImports := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation":   "application DTO mapping",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation":    "application DTO mapping",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode":   "survey/scale application QR-code use cases",
		"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel": "application query DTOs",
		"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel":  "application query DTOs",
	}
	for _, path := range []string{
		filepath.Join("handler", "questionnaire.go"),
		filepath.Join("handler", "scale.go"),
		filepath.Join("response", "display.go"),
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if replacement, ok := forbiddenImports[importPath]; ok {
				t.Fatalf("%s imports %s; survey/scale REST handlers should leave rule construction to %s", path, importPath, replacement)
			}
		}
	}
}

func TestEvaluationRESTTransportDoesNotImportWaiterInfra(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		filepath.Join("handler", "evaluation.go"),
		"routes_evaluation.go",
		"router.go",
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if importPath == "github.com/FangcunMount/qs-server/internal/apiserver/infra/waiter" {
				t.Fatalf("%s imports %s; wait-report must go through evaluation application wait service", path, importPath)
			}
		}
	}
}

func TestEvaluationRESTHandlerDoesNotImportActorAccessApplication(t *testing.T) {
	t.Parallel()

	forbiddenImports := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access":     "evaluation application access query service",
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/waiter":                 "evaluation application wait service",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment": "evaluation application service DTOs",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report":     "evaluation application service DTOs",
	}
	for _, path := range []string{
		filepath.Join("handler", "evaluation.go"),
		"routes_evaluation.go",
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if replacement, ok := forbiddenImports[importPath]; ok {
				t.Fatalf("%s imports %s; evaluation REST transport must use %s", path, importPath, replacement)
			}
		}
	}
}

func TestEvaluationRESTTransportUsesProtectedQueryFacade(t *testing.T) {
	t.Parallel()

	handlerData, err := os.ReadFile(filepath.Join("handler", "evaluation.go"))
	if err != nil {
		t.Fatal(err)
	}
	handlerText := string(handlerData)
	for _, token := range []string{
		"AssessmentAccessQueryService",
		"AssessmentWaitService",
		"ReportQueryService",
		"ScoreQueryService",
		"LoadAccessibleAssessment",
		"ScopeListAssessments",
		"ScopeListReports",
		"ScopeFactorTrend",
	} {
		if strings.Contains(handlerText, token) {
			t.Fatalf("handler/evaluation.go contains %q; REST evaluation handler must delegate access/query orchestration to AssessmentProtectedQueryService", token)
		}
	}

	routerData, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	routerText := string(routerData)
	for _, token := range []string{
		"ReportQueryService assessmentApp.ReportQueryService",
		"ScoreQueryService assessmentApp.ScoreQueryService",
		"WaitService assessmentApp.AssessmentWaitService",
		"AccessQueryService assessmentApp.AssessmentAccessQueryService",
	} {
		if strings.Contains(routerText, token) {
			t.Fatalf("router.go exposes %q; REST evaluation deps should expose the protected query facade, not query/access internals", token)
		}
	}
}
