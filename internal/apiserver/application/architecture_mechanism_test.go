package application_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// transitionalAssessmentCodePackages lists algorithm-specific extension packages allowed
// during migration to mechanism-oriented organization. Do not add new entries.
var transitionalAssessmentCodePackages = map[string]string{}

// transitionalModelFamilyPackages lists model-family packages allowed during migration.
// Evaluation and interpretation domain cores must not add new entries here.
var transitionalModelFamilyPackages = map[string]string{
	"internal/apiserver/application/evaluation/personality": "transitional: factor_classification re-export only (empty)",
}

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
		"internal/apiserver/domain/evaluation/factor_scoring",
		"internal/apiserver/domain/evaluation/factor_classification",
		"internal/apiserver/application/evaluation/factor_scoring",
		"internal/apiserver/application/evaluation/factor_classification",
		"internal/apiserver/application/evaluation/factor_norm",
		"internal/apiserver/application/evaluation/task_performance",
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
		"internal/apiserver/domain/interpretation/factor_scoring",
		"internal/apiserver/domain/interpretation/factor_classification",
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
		"internal/apiserver/application/evaluation/factor_scoring": {
			"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/scale",
		},
		"internal/apiserver/application/evaluation/factor_classification": {
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
