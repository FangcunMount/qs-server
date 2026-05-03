package application_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestApplicationsUsePortsForInfraBoundaries(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenImports := map[string]string{
		"go.mongodb.org/mongo-driver":                                 "repository ports, not Mongo driver errors",
		"github.com/FangcunMount/iam/":                                "IAM bridge ports, not generated IAM packages",
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/": "application ports, not infrastructure packages",
	}
	scanGoImports(t, filepath.Join(root, "internal", "apiserver", "application"), func(path, importPath string) {
		for forbidden, replacement := range forbiddenImports {
			if strings.HasPrefix(importPath, forbidden) {
				rel := filepath.ToSlash(mustRel(t, root, path))
				t.Fatalf("%s imports %s; application services must depend on %s", rel, importPath, replacement)
			}
		}
	})
}

func TestSurveyScaleApplicationsDoNotContainRepoBackedReadModelAdapters(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, rel := range []string{
		"internal/apiserver/application/survey/questionnaire",
		"internal/apiserver/application/survey/answersheet",
		"internal/apiserver/application/scale",
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
			for _, token := range []string{
				"RepositoryReadModel",
				"repositoryReadModel",
				"FindBaseList(",
				"FindBasePublishedList(",
				"FindSummaryList(",
				"CountWithConditions(",
			} {
				if strings.Contains(text, token) {
					t.Fatalf("%s contains %q; survey/scale application read paths must use typed read-model ports", filepath.ToSlash(mustRel(t, root, path)), token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestSurveyScaleApplicationsDoNotDependOnProceduralManagers(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, rel := range []string{
		"internal/apiserver/application/survey/questionnaire",
		"internal/apiserver/application/survey/answersheet",
		"internal/apiserver/application/scale",
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
			for _, token := range []string{
				"QuestionManager",
				"FactorManager",
			} {
				if strings.Contains(text, token) {
					t.Fatalf("%s contains %q; survey/scale application should call aggregate behavior directly", filepath.ToSlash(mustRel(t, root, path)), token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestEvaluationEngineUsesInputSnapshotPort(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	engineRoot := filepath.Join(root, "internal", "apiserver", "application", "evaluation", "engine")
	forbiddenImports := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale":  "evaluationinput snapshots",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey": "evaluationinput snapshots",
	}
	err := filepath.WalkDir(engineRoot, func(path string, entry os.DirEntry, err error) error {
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
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			for forbidden, replacement := range forbiddenImports {
				if strings.HasPrefix(importPath, forbidden) {
					t.Fatalf("%s imports %s; evaluation engine should consume %s instead of survey/scale aggregates", filepath.ToSlash(mustRel(t, root, path)), importPath, replacement)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEvaluationDoesNotUseDeprecatedRepositoryFallbacks(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, rel := range []string{
		"internal/apiserver/application/evaluation",
		"internal/apiserver/domain/evaluation",
	} {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(data)
			for _, token := range []string{
				"SaveWithEvents",
				"SaveWithAdditionalEvents",
				"SaveWithTesteeAndEvents",
				"SaveScores(",
				"NewSubmissionServiceWith",
				"NewManagementServiceWith",
				"NewScoreQueryServiceWithReadModel",
				"NewReportQueryServiceWithReadModel",
				"NewWaiterNotifyHandlerWithNotifier",
			} {
				if strings.Contains(text, token) {
					t.Fatalf("%s contains %q; evaluation must not reintroduce deprecated repository fallback methods or transition constructors", filepath.ToSlash(mustRel(t, root, path)), token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestEvaluationApplicationUsesCentralErrorMapper(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	allowedMapperDir := filepath.ToSlash(filepath.Join("internal", "apiserver", "application", "evaluation", "apperrors")) + "/"
	forbiddenImports := map[string]string{
		"github.com/FangcunMount/component-base/pkg/errors":   "evaluation/apperrors",
		"github.com/FangcunMount/qs-server/internal/pkg/code": "evaluation/apperrors",
	}
	scanGoImports(t, filepath.Join(root, "internal", "apiserver", "application", "evaluation"), func(path, importPath string) {
		rel := filepath.ToSlash(mustRel(t, root, path))
		if strings.HasSuffix(path, "_test.go") || strings.HasPrefix(rel, allowedMapperDir) {
			return
		}
		for forbidden, replacement := range forbiddenImports {
			if importPath == forbidden {
				t.Fatalf("%s imports %s; evaluation application error code mapping should be centralized in %s", rel, importPath, replacement)
			}
		}
	})
}

func TestEvaluationApplicationDoesNotDependOnActorAccessApplication(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbidden := "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	scanGoImports(t, filepath.Join(root, "internal", "apiserver", "application", "evaluation"), func(path, importPath string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		if importPath == forbidden {
			t.Fatalf("%s imports %s; evaluation application must use evaluation-owned access ports", filepath.ToSlash(mustRel(t, root, path)), importPath)
		}
	})
}

func TestEvaluationInputInfraReturnsPortErrors(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenImports := map[string]string{
		"github.com/FangcunMount/component-base/pkg/errors":   "port/evaluationinput ResolveError plus application error mapper",
		"github.com/FangcunMount/qs-server/internal/pkg/code": "port/evaluationinput FailureKind plus application error mapper",
	}
	scanGoImports(t, filepath.Join(root, "internal", "apiserver", "infra", "evaluationinput"), func(path, importPath string) {
		for forbidden, replacement := range forbiddenImports {
			if importPath == forbidden {
				t.Fatalf("%s imports %s; evaluation input infra must return %s", filepath.ToSlash(mustRel(t, root, path)), importPath, replacement)
			}
		}
	})
}

func TestEvaluationInputPortStaysIndependentFromSurveyScaleDomain(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenImports := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/":      "evaluationinput-owned snapshot DTOs",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/": "evaluationinput-owned snapshot DTOs",
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/":       "evaluationinput-owned snapshot DTOs",
		"github.com/FangcunMount/qs-server/internal/apiserver/transport/":   "evaluationinput-owned snapshot DTOs",
	}
	scanGoImports(t, filepath.Join(root, "internal", "apiserver", "port", "evaluationinput"), func(path, importPath string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		for forbidden, replacement := range forbiddenImports {
			if strings.HasPrefix(importPath, forbidden) {
				t.Fatalf("%s imports %s; evaluationinput port must stay neutral and depend on %s", filepath.ToSlash(mustRel(t, root, path)), importPath, replacement)
			}
		}
	})
}

func TestEvaluationInputInfraCommandRepoDependenciesStayInCompatibilityAdapter(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	allowedFiles := map[string]struct{}{
		"internal/apiserver/infra/evaluationinput/repository_resolver.go": {},
		"internal/apiserver/infra/evaluationinput/snapshot_mappers.go":    {},
	}
	forbiddenImports := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale":   "catalog/read-model snapshot adapters",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/": "catalog/read-model snapshot adapters",
	}
	scanGoImports(t, filepath.Join(root, "internal", "apiserver", "infra", "evaluationinput"), func(path, importPath string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		for forbidden, replacement := range forbiddenImports {
			if !strings.HasPrefix(importPath, forbidden) {
				continue
			}
			rel := filepath.ToSlash(mustRel(t, root, path))
			if _, ok := allowedFiles[rel]; ok {
				return
			}
			t.Fatalf("%s imports %s; command repository/domain dependencies must stay isolated in compatibility adapter files until replaced by %s", rel, importPath, replacement)
		}
	})
}

func TestEvaluationDomainDoesNotKeepReadPaginationValueObjects(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, rel := range []string{
		"internal/apiserver/domain/evaluation/assessment",
		"internal/apiserver/domain/evaluation/report",
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
			if strings.Contains(string(data), "type Pagination") || strings.Contains(string(data), "NewPagination") {
				t.Fatalf("%s contains pagination read-model value object; pagination belongs to application/read-model ports", filepath.ToSlash(mustRel(t, root, path)))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestEvaluationDomainDoesNotDependOnOuterLayersOrSiblingAggregates(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenImports := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/application/":                 "application error mapping/use cases",
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/":                       "infrastructure adapters",
		"github.com/FangcunMount/qs-server/internal/apiserver/transport/":                   "transport adapters",
		"github.com/FangcunMount/component-base/pkg/logger":                                 "application/infra observability",
		"github.com/FangcunMount/component-base/pkg/errors":                                 "domain-native errors",
		"github.com/FangcunMount/component-base/pkg/code":                                   "domain-native errors",
		"github.com/FangcunMount/qs-server/internal/pkg/code":                               "application API error mapping",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale":                 "evaluation-local snapshots/value objects",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey":                "evaluationinput snapshots",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment": "report-local snapshots/value objects",
	}
	scanGoImports(t, filepath.Join(root, "internal", "apiserver", "domain", "evaluation"), func(path, importPath string) {
		for forbidden, replacement := range forbiddenImports {
			if strings.HasPrefix(importPath, forbidden) {
				rel := filepath.ToSlash(mustRel(t, root, path))
				if strings.Contains(rel, "domain/evaluation/assessment/") && forbidden == "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment" {
					continue
				}
				t.Fatalf("%s imports %s; evaluation domain should depend on %s", rel, importPath, replacement)
			}
		}
	})
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
