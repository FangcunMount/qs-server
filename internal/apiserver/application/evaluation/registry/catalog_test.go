package registry_test

import (
	"testing"

	evalregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestDefaultEvaluationCatalogPiecesAlign(t *testing.T) {
	t.Parallel()

	descs := evalregistry.DefaultEvaluationDescriptors()
	if len(descs) == 0 {
		t.Fatal("descriptors are empty")
	}
	typologyRegistry, err := evalregistry.DefaultTypologyRegistry()
	if err != nil {
		t.Fatalf("DefaultTypologyRegistry: %v", err)
	}
	if typologyRegistry.Len() != len(evalregistry.DefaultTypologyModules()) {
		t.Fatalf("typology registry len = %d, want %d", typologyRegistry.Len(), len(evalregistry.DefaultTypologyModules()))
	}
	runtimeRegistry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatalf("DefaultRuntimeDescriptorRegistry: %v", err)
	}
	if runtimeRegistry.Len() != 4 {
		t.Fatalf("runtime registry len = %d, want 4", runtimeRegistry.Len())
	}
	foundTypology := false
	for _, desc := range descs {
		if desc.Algorithm == modelcatalog.AlgorithmPersonalityTypology {
			foundTypology = true
		}
	}
	if !foundTypology {
		t.Fatal("typology descriptor missing from default descriptors")
	}
}
