package adapter_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter"
)

func TestDefaultRegistryMatchesBuiltInTypologyAlgorithms(t *testing.T) {
	registry := adapter.DefaultRegistry()
	for _, algorithm := range []modelcatalog.Algorithm{
		modelcatalog.AlgorithmMBTI,
		modelcatalog.AlgorithmSBTI,
		modelcatalog.AlgorithmBigFive,
	} {
		if _, err := registry.Resolve(algorithm); err != nil {
			t.Fatalf("Resolve(%s): %v", algorithm, err)
		}
	}
	if _, err := registry.Resolve(modelcatalog.Algorithm("typology_unknown")); err == nil {
		t.Fatal("expected unknown adapter to be unsupported")
	}
}
