package adapter_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter"
)

func TestDefaultRegistryMatchesModelDescriptors(t *testing.T) {
	descs := evaldomain.DefaultModelDescriptors()
	registry := adapter.DefaultRegistry()
	for _, desc := range descs {
		if desc.Kind != evaldomain.ModelKindTypology {
			continue
		}
		if _, err := registry.Resolve(desc.Algorithm); err != nil {
			t.Fatalf("Resolve(%s): %v", desc.Algorithm, err)
		}
	}
	if _, err := registry.Resolve(assessmentmodel.AlgorithmBigFive); err == nil {
		t.Fatal("expected BigFive adapter to be unsupported")
	}
}
