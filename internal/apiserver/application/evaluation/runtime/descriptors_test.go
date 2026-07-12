package runtime_test

import (
	"testing"

	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestExecutionPathsFromRegistryMatchesMaterializationOrder(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatalf("DefaultRuntimeDescriptorRegistry: %v", err)
	}
	paths, err := evalruntime.ExecutionPathsFromRegistry(registry)
	if err != nil {
		t.Fatalf("ExecutionPathsFromRegistry: %v", err)
	}
	want := []modelcatalog.ExecutionPath{
		modelcatalog.ExecutionPathScaleDescriptor,
		modelcatalog.ExecutionPathTypologyDescriptor,
		modelcatalog.ExecutionPathBehavioralRatingDescriptor,
		modelcatalog.ExecutionPathCognitiveDescriptor,
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	for i, path := range want {
		if paths[i] != path {
			t.Fatalf("paths[%d] = %s, want %s", i, paths[i], path)
		}
	}
}
