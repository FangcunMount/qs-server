package adapter_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter"
)

func TestDefaultRegistryMatchesBuiltInTypologyAlgorithms(t *testing.T) {
	registry := adapter.DefaultRegistry()
	for _, algorithm := range []assessmentmodel.Algorithm{
		assessmentmodel.AlgorithmMBTI,
		assessmentmodel.AlgorithmSBTI,
		assessmentmodel.AlgorithmBigFive,
	} {
		if _, err := registry.Resolve(algorithm); err != nil {
			t.Fatalf("Resolve(%s): %v", algorithm, err)
		}
	}
	if _, err := registry.Resolve(assessmentmodel.Algorithm("typology_unknown")); err == nil {
		t.Fatal("expected unknown adapter to be unsupported")
	}
}
