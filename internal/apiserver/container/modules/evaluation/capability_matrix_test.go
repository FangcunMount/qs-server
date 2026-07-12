package evaluation_test

import (
	"testing"

	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestEvaluationModuleRegistersOnlyDeclaredDescriptorFamilies(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatalf("DefaultRuntimeDescriptorRegistry() error = %v", err)
	}
	paths, err := evalruntime.ExecutionPathsFromRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	if registry.Len() != len(paths) {
		t.Fatalf("runtime descriptor count = %d, paths = %d", registry.Len(), len(paths))
	}
	for _, kind := range domain.RuntimeExecutableKinds() {
		capability, ok := domain.FamilyCapabilityByKind(kind)
		family, familyOK := evalpipeline.AlgorithmFamilyFromModelKind(evalpipeline.ModelKind(kind))
		if !ok || !capability.RuntimeExecutable || !familyOK || !registry.HasAlgorithmFamily(family) {
			t.Fatalf("runtime descriptor missing for kind %s", kind)
		}
	}
	if registry.HasAlgorithmFamily(domain.AlgorithmFamily("custom")) {
		t.Fatal("custom runtime descriptor must not be registered")
	}
}
