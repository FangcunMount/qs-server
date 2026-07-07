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
	reportPaths, err := evalruntime.RegisteredReportBuilderPaths()
	if err != nil {
		t.Fatalf("RegisteredReportBuilderPaths: %v", err)
	}
	scorePaths, err := evalruntime.RegisteredScoreProjectorPaths()
	if err != nil {
		t.Fatalf("RegisteredScoreProjectorPaths: %v", err)
	}
	assertSamePaths(t, "evaluator", paths, evaluatorPaths)
	assertSamePaths(t, "report builder", paths, reportPaths)
	wantScore := []modelcatalog.ExecutionPath{
		modelcatalog.ExecutionPathScaleDescriptor,
		modelcatalog.ExecutionPathBehavioralRatingDescriptor,
		modelcatalog.ExecutionPathCognitiveDescriptor,
	}
	assertSamePaths(t, "score projector", wantScore, scorePaths)
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
