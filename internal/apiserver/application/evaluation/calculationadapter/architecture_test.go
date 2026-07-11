package calculationadapter_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCalculationAdapterSharesGenericOutcomeBridge(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	required := map[string]struct{}{
		"outcome.go":         {},
		"score.go":           {},
		"score_node.go":      {},
		"scoring_outcome.go": {},
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		delete(required, entry.Name())
	}
	for missing := range required {
		t.Fatalf("missing shared calculationadapter file %s; norming/scoring should reuse outcome.go and score_node.go", missing)
	}
}

func TestMechanismExecutorsImportCalculationAdapter(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	mechanisms := []string{"scoring", "norming", "task_performance"}
	for _, mechanism := range mechanisms {
		dir := filepath.Join(root, "internal/apiserver/application/evaluation/registry/mechanisms", mechanism)
		found := false
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), "calculationadapter") {
				found = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("mechanism %s must import calculationadapter", mechanism)
		}
	}
}

func TestCalculationAdapterUsesCanonicalExecutionWithoutAssessmentOutcomeBridge(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, forbidden := range []string{"domain/evaluation/assessment", "AssessmentOutcomeFromExecution", "ExecutionFromAssessmentOutcome"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains %q; calculation adapter must translate Execution directly", entry.Name(), forbidden)
			}
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
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
