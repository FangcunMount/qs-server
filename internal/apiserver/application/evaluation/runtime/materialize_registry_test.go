package runtime_test

import (
	"testing"

	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestMaterializeFactoriesAlignWithRuntimeDescriptorRegistry(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatalf("DefaultRuntimeDescriptorRegistry: %v", err)
	}
	paths, err := evalruntime.ExecutionPathsFromRegistry(registry)
	if err != nil {
		t.Fatalf("ExecutionPathsFromRegistry: %v", err)
	}
	evaluatorPaths, err := evalruntime.RegisteredEvaluatorPaths()
	if err != nil {
		t.Fatalf("RegisteredEvaluatorPaths: %v", err)
	}
	assertSamePaths(t, "evaluator", paths, evaluatorPaths)
}

func assertSamePaths(t *testing.T, name string, want, got []modelcatalog.ExecutionPath) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s path count = %d, want %d (got=%#v want=%#v)", name, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s paths[%d] = %s, want %s", name, i, got[i], want[i])
		}
	}
}
