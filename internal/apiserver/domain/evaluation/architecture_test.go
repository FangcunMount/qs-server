package evaluation_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEvaluationRootOnlyAllowsExecutionPackages(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	evalRoot := filepath.Join(root, "internal", "apiserver", "domain", "evaluation")
	allowedTopLevel := map[string]struct{}{
		"assessment": {},
		"run":        {},
		"input":      {},
		"policy":     {},
		"pipeline":   {},
		"event":      {},
	}
	forbiddenAssessmentCodeFiles := []string{
		"score_mbti.go",
		"score_sbti.go",
	}
	err := filepath.WalkDir(evalRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path == evalRoot {
				return nil
			}
			rel, _ := filepath.Rel(evalRoot, path)
			if !strings.Contains(rel, string(filepath.Separator)) {
				if _, ok := allowedTopLevel[entry.Name()]; !ok {
					t.Fatalf("unexpected top-level evaluation package %q; allowed: assessment/run/input/policy/pipeline/event", entry.Name())
				}
			}
			if entry.Name() == "norming" || entry.Name() == "task_performance" {
				t.Fatalf("placeholder mechanism package %q must not exist under domain/evaluation", entry.Name())
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		base := strings.ToLower(filepath.Base(path))
		for _, token := range forbiddenAssessmentCodeFiles {
			if strings.Contains(base, token) {
				t.Fatalf("%s leaks assessment code filename %q under domain/evaluation", path, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDomainAssessmentDoesNotDefineApplyEvaluation(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	scanRoot := filepath.Join(root, "internal", "apiserver", "domain", "evaluation", "assessment")
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
		if strings.Contains(string(data), "func (a *Assessment) ApplyEvaluation(") {
			t.Fatalf("%s defines ApplyEvaluation; use ApplyOutcome instead", path)
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
