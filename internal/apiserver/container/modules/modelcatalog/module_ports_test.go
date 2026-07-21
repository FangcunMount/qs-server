package modelcatalog

import (
	"testing"

	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestEvaluationCatalogExportsRuntimeExecutionPaths(t *testing.T) {
	catalog, err := ExportEvaluationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	want := []modelcatalog.ExecutionPath{
		modelcatalog.ExecutionPathScaleDescriptor,
		modelcatalog.ExecutionPathTypologyDescriptor,
		modelcatalog.ExecutionPathBehavioralRatingDescriptor,
		modelcatalog.ExecutionPathCognitiveDescriptor,
	}
	// Exact (AlgorithmFamily, DecisionKind) registration intentionally creates
	// seven descriptors that collapse to four execution paths.
	if catalog.RuntimeDescriptorRegistry == nil || catalog.RuntimeDescriptorRegistry.Len() != 7 {
		t.Fatalf("runtime registry = %#v", catalog.RuntimeDescriptorRegistry)
	}
	paths, err := evalruntime.ExecutionPathsFromRegistry(catalog.RuntimeDescriptorRegistry)
	if err != nil {
		t.Fatal(err)
	}
	paths = evalruntime.FilterExecutablePaths(paths)
	if len(paths) != len(want) {
		t.Fatalf("paths = %#v", paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("path[%d] = %s, want %s", i, paths[i], want[i])
		}
	}
}
