package application_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationEvaluationScoringWritePathLivesInOutcome(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "internal/apiserver/application/evaluation/scoring")); !os.IsNotExist(err) {
		t.Fatalf("application/evaluation/scoring must be removed; use outcome/scoring for write path")
	}
	required := filepath.Join(root, "internal/apiserver/application/evaluation/outcome/scoring")
	if _, err := os.Stat(required); err != nil {
		t.Fatalf("missing outcome scoring write path: %v", err)
	}
}

func TestMechanismTypologyDoesNotAddAssessmentCodeReportFiles(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	typologyRoot := filepath.Join(root, "internal/apiserver/application/evaluation/registry/mechanisms/typology")
	forbiddenPrefixes := []string{"report_mbti", "report_sbti", "report_bigfive"}
	err := filepath.WalkDir(typologyRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "legacy" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		base := strings.ToLower(filepath.Base(path))
		for _, prefix := range forbiddenPrefixes {
			if strings.HasPrefix(base, prefix) {
				t.Fatalf("%s uses forbidden assessment-code report file name; use report_builder/report_generic + legacy adapters",
					filepath.ToSlash(mustRel(t, root, path)))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEvaluationApplicationDoesNotAddModelFamilyTopLevelDirs(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	evalRoot := filepath.Join(root, "internal/apiserver/application/evaluation")
	allowedTopLevel := map[string]struct{}{
		"assessment": {}, "calculationadapter": {}, "execute": {}, "outcome": {},
		"registry": {}, "runtime": {}, "runquery": {}, "apperrors": {},
	}
	err := filepath.WalkDir(evalRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		if rel != "internal/apiserver/application/evaluation" {
			return nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, child := range entries {
			if !child.IsDir() {
				continue
			}
			name := child.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			if _, ok := allowedTopLevel[name]; ok {
				continue
			}
			if isForbiddenModelFamilyDirName(name) {
				t.Fatalf("%s/%s is forbidden model-family directory under application/evaluation; use registry/mechanisms",
					rel, name)
			}
		}
		return filepath.SkipDir
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMechanismProductionCodeDoesNotImportDomainMBTIPatternSymbols(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	mechanismsRoot := filepath.Join(root, "internal/apiserver/application/evaluation/registry/mechanisms")
	forbiddenTokens := []string{
		"MBTIResultDetailFromPayload",
		"SBTIResultDetailFromPayload",
		"BigFiveResultDetailFromPayload",
		"patterns.MBTI",
		"patterns.SBTI",
	}
	err := filepath.WalkDir(mechanismsRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "legacy" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, token := range forbiddenTokens {
			if strings.Contains(text, token) {
				t.Fatalf("%s contains %q; use mechanism-neutral detail APIs or typology/legacy adapters",
					filepath.ToSlash(mustRel(t, root, path)), token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
