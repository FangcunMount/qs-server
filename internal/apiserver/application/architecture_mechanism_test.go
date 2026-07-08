package application_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// transitionalAssessment编码包 lists 算法-特定 extension 包 allowed。
// 在 迁移 到 面向机制 organization. Do 不 add new entries。
var transitionalAssessmentCodePackages = map[string]string{}

// transitional模型家族包 lists model-家族 包 allowed 在 迁移。
// Evaluation 和 interpretation 领域 cores 不得 add new entries here。
var transitionalModelFamilyPackages = map[string]string{}

var forbiddenAssessmentCodeDirNames = []string{
	"brief2", "spm", "mbti", "sbti", "bigfive", "conners", "snap_iv", "snapiv", "phq9", "gad7",
}

var forbiddenEvaluationInterpretationFamilyDirNames = []string{
	"personality", "scale", "behavioral_rating", "cognitive",
}

func TestDomainAndApplicationDoNotAddAssessmentCodeNamedPackages(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, relRoot := range []string{
		"internal/apiserver/domain",
		"internal/apiserver/application",
	} {
		scanRoot := filepath.Join(root, filepath.FromSlash(relRoot))
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				return nil
			}
			name := entry.Name()
			if !isForbiddenAssessmentCodeDirName(name) {
				return nil
			}
			rel := filepath.ToSlash(mustRel(t, root, path))
			if reason, ok := transitionalAssessmentCodePackages[rel]; ok {
				t.Logf("allowed transitional package %s (%s)", rel, reason)
				return filepath.SkipDir
			}
			t.Fatalf("%s uses forbidden assessment code directory name %q; organize by mechanism, not assessment code", rel, name)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestEvaluationAndInterpretationDomainCoresDoNotAddModelFamilyPackages(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, relRoot := range []string{
		"internal/apiserver/domain/evaluation",
		"internal/apiserver/domain/interpretation",
	} {
		scanRoot := filepath.Join(root, filepath.FromSlash(relRoot))
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				return nil
			}
			rel := filepath.ToSlash(mustRel(t, root, path))
			if rel == relRoot {
				return nil
			}
			name := entry.Name()
			if !isForbiddenModelFamilyDirName(name) {
				return nil
			}
			if reason, ok := transitionalModelFamilyPackages[rel]; ok {
				t.Logf("allowed transitional package %s (%s)", rel, reason)
				return filepath.SkipDir
			}
			t.Fatalf("%s uses forbidden model-family directory name %q under evaluation/interpretation core; organize by mechanism (pipeline/report/template/builder), not model family", rel, name)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestMechanismOrientedEvaluationPackagesExist(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	required := []string{
		"internal/apiserver/domain/evaluation/pipeline",
		"internal/apiserver/domain/evaluation/input",
		"internal/apiserver/domain/evaluation/policy",
		"internal/apiserver/domain/evaluation/run",
		"internal/apiserver/domain/evaluation/event",
		"internal/apiserver/domain/evaluation/scoring",
		"internal/apiserver/domain/evaluation/typology",
		"internal/apiserver/domain/calculation/scoring",
		"internal/apiserver/domain/calculation/classification",
		"internal/apiserver/application/evaluation/registry",
		"internal/apiserver/application/evaluation/registry/mechanisms/scoring",
		"internal/apiserver/application/evaluation/registry/mechanisms/typology",
		"internal/apiserver/application/evaluation/registry/mechanisms/norming",
		"internal/apiserver/application/evaluation/registry/mechanisms/task_performance",
	}
	for _, rel := range required {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("required mechanism package missing: %s (%v)", rel, err)
		}
	}
}

func TestMechanismOrientedInterpretationPackagesExist(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	required := []string{
		"internal/apiserver/domain/interpretation/report",
		"internal/apiserver/domain/interpretation/template",
		"internal/apiserver/domain/interpretation/builder",
		"internal/apiserver/domain/interpretation/rule",
		"internal/apiserver/domain/interpretation/policy",
		"internal/apiserver/domain/interpretation/scoring",
		"internal/apiserver/domain/interpretation/typology",
	}
	for _, rel := range required {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("required mechanism package missing: %s (%v)", rel, err)
		}
	}
}

func isForbiddenAssessmentCodeDirName(name string) bool {
	lower := strings.ToLower(name)
	for _, forbidden := range forbiddenAssessmentCodeDirNames {
		if lower == forbidden {
			return true
		}
	}
	return false
}

func isForbiddenModelFamilyDirName(name string) bool {
	lower := strings.ToLower(name)
	for _, forbidden := range forbiddenEvaluationInterpretationFamilyDirNames {
		if lower == forbidden {
			return true
		}
	}
	return false
}

func TestExecutionPathRoutingLivesInPipelinePackage(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenTokens := []string{
		"func executionPathForFamily(",
		"func algorithmFamilyFromModelKind(",
	}
	allowedFiles := map[string]struct{}{
		"internal/apiserver/domain/evaluation/pipeline/resolve.go": {},
	}
	scanRoots := []string{
		"internal/apiserver/domain/evaluation",
		"internal/apiserver/application/evaluation",
		"internal/apiserver/infra/evaluationinput",
	}
	for _, relRoot := range scanRoots {
		scanRoot := filepath.Join(root, filepath.FromSlash(relRoot))
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel := filepath.ToSlash(mustRel(t, root, path))
			if _, ok := allowedFiles[rel]; ok {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(data)
			for _, token := range forbiddenTokens {
				if strings.Contains(text, token) {
					t.Fatalf("%s contains %q; route ExecutionPath mapping through domain/evaluation/pipeline only", rel, token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestApplicationFactorMechanismsUseDomainEntryPackages(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbiddenImports := map[string][]string{
		"internal/apiserver/application/evaluation/registry/mechanisms/scoring": {
			"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/scale",
		},
		"internal/apiserver/application/evaluation/registry/mechanisms/typology": {
			"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality",
		},
	}
	for relDir, forbidden := range forbiddenImports {
		scanRoot := filepath.Join(root, relDir)
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
			for _, imp := range forbidden {
				if strings.Contains(text, imp) {
					t.Fatalf("%s imports %s; use domain mechanism entry package instead", filepath.ToSlash(mustRel(t, root, path)), imp)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestApplicationDoesNotImportLegacyFactorMechanismHosts(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	legacyImports := []string{
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_scoring",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_classification",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_norm",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/task_performance",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/factor_scoring",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/factor_classification",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/factor_norm",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/range_scoring",
	}
	allowedImporterPrefixes := []string{
		"internal/apiserver/application/evaluation/registry/mechanisms/",
		"internal/apiserver/application/evaluation/registry/",
		"internal/apiserver/application/evaluation/runtime/",
		"internal/apiserver/characterization/",
	}
	scanRoot := filepath.Join(root, "internal", "apiserver", "application")
	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		for _, prefix := range allowedImporterPrefixes {
			if strings.HasPrefix(rel, prefix) {
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, imp := range legacyImports {
			if strings.Contains(text, imp) {
				t.Fatalf("%s imports legacy mechanism host %s; use application/evaluation/registry", rel, imp)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
