package reporting_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildersDoNotDependOnEvaluationCompatibilityInput(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	root := filepath.Dir(file)
	paths := []string{
		filepath.Join(root, "factor_scoring_report.go"),
		filepath.Join(root, "norm_task_report.go"),
		filepath.Join(root, "registry", "report_builder.go"),
		filepath.Join(root, "typology", "report_builder.go"),
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"application/evaluation/outcome",
			"domain/evaluation/assessment",
			"port/evaluationinput",
		} {
			if strings.Contains(string(data), forbidden) {
				t.Fatalf("%s must not import %s", filepath.Base(path), forbidden)
			}
		}
	}
}
