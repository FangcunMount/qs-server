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
		"internal/apiserver/application/modelcatalog/behavior/scale",
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
		"internal/apiserver/application/modelcatalog/behavior/scale",
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

func TestEvaluationExecuteUsesInputSnapshotPort(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	executeRoot := filepath.Join(root, "internal", "apiserver", "application", "evaluation", "execute")
	forbiddenImports := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition": "evaluationinput snapshots",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey":                        "evaluationinput snapshots",
	}
	err := filepath.WalkDir(executeRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			for forbidden, replacement := range forbiddenImports {
				if strings.HasPrefix(importPath, forbidden) {
					t.Fatalf("%s imports %s; generic evaluation execute should consume %s instead of survey/scale aggregates", rel, importPath, replacement)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestScaleEvaluationExecutorDoesNotImportLegacyPipeline(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	scanGoImports(t, filepath.Join(root, "internal", "apiserver", "application", "evaluation", "registry", "mechanisms", "scoring"), func(path, importPath string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		if strings.Contains(importPath, "/application/evaluation/engine") {
			t.Fatalf("%s imports %s; scoring executor must not wrap legacy evaluation pipeline", filepath.ToSlash(mustRel(t, root, path)), importPath)
		}
	})
}

func TestInterpretationReportingDoesNotOwnScaleRules(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenImports := []string{
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition",
		"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine",
	}
	scanGoImports(t, filepath.Join(root, "internal", "apiserver", "application", "interpretation", "reporting"), func(path, importPath string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		for _, forbidden := range forbiddenImports {
			if strings.HasPrefix(importPath, forbidden) {
				t.Fatalf("%s imports %s; report writer must orchestrate outputs without owning scale rules", filepath.ToSlash(mustRel(t, root, path)), importPath)
			}
		}
	})
}

func TestScoringDefinitionDoesNotModelMBTIAsCategory(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "domain", "modelcatalog", "scoring", "definition", "types.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{`CategoryMBTI`, `Category = "mbti"`} {
		if strings.Contains(text, token) {
			t.Fatalf("%s contains %q; MBTI must be a peer interpretation model, not a scale category", filepath.ToSlash(mustRel(t, root, path)), token)
		}
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

func TestEvaluationExecuteKeepsScaleCompatibilityIsolated(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	dir := filepath.Join(root, "internal", "apiserver", "application", "evaluation", "execute")
	allowed := map[string]struct{}{
		filepath.ToSlash(filepath.Join("internal", "apiserver", "application", "evaluation", "execute", "scale_compatibility.go")): {},
	}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, token := range []string{"MedicalScale", "ScalePayload("} {
			if _, ok := allowed[rel]; !ok && strings.Contains(text, token) {
				t.Fatalf("%s contains %q; execute layer scale compatibility must stay isolated in scale_compatibility.go", rel, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/":      "ruleset typed payload aliases only",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/": "evaluationinput-owned snapshot DTOs",
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/":       "evaluationinput-owned snapshot DTOs",
		"github.com/FangcunMount/qs-server/internal/apiserver/transport/":   "evaluationinput-owned snapshot DTOs",
	}
	scanGoImports(t, filepath.Join(root, "internal", "apiserver", "port", "evaluationinput"), func(path, importPath string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		if isEvaluationRulesetPayloadImport(importPath) {
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
		"internal/apiserver/infra/evaluationinput/repository_resolver.go":  {},
		"internal/apiserver/infra/evaluationinput/snapshot_mappers.go":     {},
		"internal/apiserver/infra/evaluationinput/scale_binding_source.go": {},
	}
	forbiddenImports := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition": "catalog/read-model snapshot adapters",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/":                       "catalog/read-model snapshot adapters",
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
		"internal/apiserver/domain/interpretation",
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
		"github.com/FangcunMount/qs-server/internal/apiserver/application/":                         "application error mapping/use cases",
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/":                               "infrastructure adapters",
		"github.com/FangcunMount/qs-server/internal/apiserver/transport/":                           "transport adapters",
		"github.com/FangcunMount/component-base/pkg/logger":                                         "application/infra observability",
		"github.com/FangcunMount/component-base/pkg/errors":                                         "domain-native errors",
		"github.com/FangcunMount/component-base/pkg/code":                                           "domain-native errors",
		"github.com/FangcunMount/qs-server/internal/pkg/code":                                       "application API error mapping",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition": "evaluation-local snapshots/value objects",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey":                        "evaluationinput snapshots",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment":         "report-local snapshots/value objects",
	}
	scanGoImports(t, filepath.Join(root, "internal", "apiserver", "domain", "evaluation"), func(path, importPath string) {
		if isEvaluationRootPackageGoFile(root, path) {
			if isEvaluationRulesetPayloadImport(importPath) {
				return
			}
		}
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

func TestCalculationDomainStaysStatelessKernel(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbidden := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment": "its own calculation.Result types",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog":          "neutral calculation inputs (ScoreNode); callers translate factor/model-catalog assets",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey":                "neutral calculation inputs; callers translate question assets",
	}
	scanGoImports(t, filepath.Join(root, "internal", "apiserver", "domain", "calculation"), func(path, importPath string) {
		rel := filepath.ToSlash(mustRel(t, root, path))
		if strings.Contains(rel, "/specialrule/") {
			return
		}
		for prefix, replacement := range forbidden {
			if strings.HasPrefix(importPath, prefix) {
				t.Fatalf("%s imports %s; calculation kernel must stay domain-asset free and use %s", filepath.ToSlash(mustRel(t, root, path)), importPath, replacement)
			}
		}
	})
}

func isEvaluationRootPackageGoFile(root, path string) bool {
	evaluationRoot := filepath.Join(root, "internal", "apiserver", "domain", "evaluation")
	rel, err := filepath.Rel(evaluationRoot, path)
	if err != nil || strings.Contains(rel, string(os.PathSeparator)) {
		return false
	}
	return strings.HasSuffix(path, ".go")
}

func isEvaluationRulesetPayloadImport(importPath string) bool {
	if importPath == "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog" {
		return true
	}
	if importPath == "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation" {
		return true
	}
	for _, allowed := range []string{
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming/snapshot",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/taskperformance/snapshot",
	} {
		if importPath == allowed || strings.HasPrefix(importPath, allowed+"/") {
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

func TestReportDomainDoesNotUseAlgorithmNamedTopLevelPackages(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenDirs := []string{
		filepath.Join(root, "internal", "apiserver", "domain", "interpretation", "mbti"),
		filepath.Join(root, "internal", "apiserver", "domain", "interpretation", "sbti"),
	}
	for _, dir := range forbiddenDirs {
		if _, err := os.Stat(dir); err == nil {
			t.Fatalf("%s must not exist; typology report assembly belongs in domain/interpretation/typology/patterns", filepath.ToSlash(mustRel(t, root, dir)))
		}
	}

	forbiddenImports := []string{
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/mbti",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/sbti",
	}
	scanGoImports(t, filepath.Join(root, "internal", "apiserver"), func(path, importPath string) {
		for _, forbidden := range forbiddenImports {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf("%s imports %s; use domain/interpretation/typology/patterns instead", filepath.ToSlash(mustRel(t, root, path)), importPath)
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

func mustRel(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}

func TestApplicationLayerDoesNotReferenceLegacyRuleSetTypes(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenTokens := []string{
		"RuleSetSnapshot",
		"RuleSetDefinition",
		"RuleSetKind",
		"domain/ruleset",
	}
	scanRoot := filepath.Join(root, "internal", "apiserver", "application")
	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.Base(path) == "architecture_test.go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, token := range forbiddenTokens {
			if strings.Contains(text, token) {
				t.Fatalf("%s contains %q; application must use assessmentmodel v2 types", filepath.ToSlash(mustRel(t, root, path)), token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewWritesDoNotUseMigrationKinds(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenTokens := []string{
		"KindMBTIMigration",
		"KindSBTIMigration",
		"RuleSetKindMBTI",
		"RuleSetKindSBTI",
	}
	allowedRelPrefixes := []string{
		"internal/apiserver/infra/",
		"internal/apiserver/characterization/",
		"internal/pkg/migration/",
		"scripts/",
	}
	scanRoots := []string{
		filepath.Join(root, "internal", "apiserver", "application"),
		filepath.Join(root, "internal", "apiserver", "transport"),
		filepath.Join(root, "internal", "apiserver", "domain", "evaluation"),
	}
	for _, scanRoot := range scanRoots {
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if filepath.Base(path) == "architecture_test.go" {
				return nil
			}
			rel := filepath.ToSlash(mustRel(t, root, path))
			for _, prefix := range allowedRelPrefixes {
				if strings.HasPrefix(rel, prefix) {
					return nil
				}
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(data)
			for _, token := range forbiddenTokens {
				if strings.Contains(text, token) {
					t.Fatalf("%s contains %q; migration kinds belong in legacy adapters only", rel, token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestScaleModelDoesNotContainOtherModelFamilyConcepts(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenTokens := []string{
		"MBTI",
		"SBTI",
		"BigFive",
		"BRIEF",
		"SPM",
		"Typology",
		"TScore",
		"Percentile",
		"Ability",
		"SubKindTrait",
	}
	scaleRoots := []string{
		filepath.Join(root, "internal", "apiserver", "domain", "modelcatalog", "scoring"),
		filepath.Join(root, "internal", "apiserver", "domain", "interpretation", "scoring"),
		filepath.Join(root, "internal", "apiserver", "application", "evaluation", "registry", "mechanisms", "scoring"),
	}
	for _, scanRoot := range scaleRoots {
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
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
			for _, token := range forbiddenTokens {
				if strings.Contains(text, token) {
					t.Fatalf("%s contains %q; scale packages must stay scale-only", filepath.ToSlash(mustRel(t, root, path)), token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestApplicationEvaluationPrefersAssessmentOutcomeOverLegacyResult(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenTokens := []string{
		"NewEvaluationResult(",
		"NewModelEvaluationResult(",
	}
	allowedRelPrefixes := []string{
		"internal/apiserver/characterization/",
		"internal/apiserver/application/evaluation/registry/mechanisms/scoring/outcome_mapper.go",
		"internal/apiserver/application/evaluation/outcome/legacy.go",
	}
	scanRoot := filepath.Join(root, "internal", "apiserver", "application", "evaluation")
	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		for _, prefix := range allowedRelPrefixes {
			if strings.HasPrefix(rel, prefix) {
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, token := range forbiddenTokens {
			if strings.Contains(text, token) {
				t.Fatalf("%s contains %q; application evaluation write paths must use AssessmentOutcome as the primary model", rel, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplicationEvaluationExecuteDoesNotExposeFlatKindRouting(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	dir := filepath.Join(root, "internal", "apiserver", "application", "evaluation", "execute")
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
		if strings.Contains(string(data), "ResolveLegacyKind") {
			t.Fatalf("%s contains ResolveLegacyKind; routing must use EvaluatorKey only", filepath.ToSlash(mustRel(t, root, path)))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplicationEvaluationLegacyResultAccessIsBoundaryOnly(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	allowedRelFiles := map[string]struct{}{
		"internal/apiserver/application/evaluation/outcome/legacy.go": {},
	}
	scanRoot := filepath.Join(root, "internal", "apiserver", "application", "evaluation")
	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		if _, ok := allowedRelFiles[rel]; ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "LegacyResult()") {
			t.Fatalf("%s contains LegacyResult(); legacy projection must stay in boundary files only", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplicationEvaluationToEvaluationResultIsBoundaryOnly(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	allowedRelFiles := map[string]struct{}{
		"internal/apiserver/application/evaluation/outcome/legacy.go": {},
	}
	scanRoot := filepath.Join(root, "internal", "apiserver", "application", "evaluation")
	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		if _, ok := allowedRelFiles[rel]; ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "ToEvaluationResult()") {
			t.Fatalf("%s contains ToEvaluationResult(); legacy projection must stay in boundary files only", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplicationEvaluationAssessmentOutcomeFromEvaluationResultIsBoundaryOnly(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	allowedRelFiles := map[string]struct{}{
		"internal/apiserver/application/evaluation/outcome/legacy.go": {},
	}
	scanRoot := filepath.Join(root, "internal", "apiserver", "application", "evaluation")
	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		if _, ok := allowedRelFiles[rel]; ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "AssessmentOutcomeFromEvaluationResult(") {
			t.Fatalf("%s contains AssessmentOutcomeFromEvaluationResult(); adapter must stay in boundary files only", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplicationEvaluationDoesNotCallApplyEvaluation(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	scanRoot := filepath.Join(root, "internal", "apiserver", "application", "evaluation")
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
		if strings.Contains(string(data), ".ApplyEvaluation(") {
			t.Fatalf("%s calls ApplyEvaluation; application must use ApplyOutcome via writer", filepath.ToSlash(mustRel(t, root, path)))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEvaluationInputPortTypologySnapshotsUseV2Kind(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "port", "evaluationinput", "input.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "Kind:           EvaluationModelKindPersonality") {
		t.Fatal("port/evaluationinput typology snapshots must set Kind=personality")
	}
	for _, want := range []string{
		"func (TypologyModelPayload) RuleSetKind() EvaluationModelKind {\n\treturn EvaluationModelKindPersonality",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("port/evaluationinput missing v2 RuleSetKind: %q", want)
		}
	}
}
