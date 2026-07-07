package typology

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func TestOutcomeAdapterRegistryRejectsUnknownAdapter(t *testing.T) {
	_, err := DefaultOutcomeAdapterRegistry().Assemble(
		modeltypology.DetailAdapterKey("custom_unknown"),
		assessment.EvaluationModelRef{},
		evaluationtypology.ScoringResult{},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported detail adapter key") {
		t.Fatalf("Assemble error = %v, want unsupported detail adapter key", err)
	}
}

func TestDefaultOutcomeAdapterRegistryOnlyRegistersMechanismKeys(t *testing.T) {
	registry := DefaultOutcomeAdapterRegistry()
	if registry.Len() != 2 {
		t.Fatalf("registry len = %d, want 2 mechanism adapters", registry.Len())
	}
	for _, key := range []modeltypology.DetailAdapterKey{
		modeltypology.DetailAdapterMBTI,
		modeltypology.DetailAdapterSBTI,
		modeltypology.DetailAdapterBigFive,
	} {
		if _, err := registry.Assemble(key, assessment.EvaluationModelRef{}, evaluationtypology.ScoringResult{}); err == nil {
			t.Fatalf("default registry should not register legacy adapter %s", key)
		}
	}
}

func TestRegisterLegacyOutcomeAdaptersRestoresLegacyKeys(t *testing.T) {
	registry := RegisterLegacyOutcomeAdapters(DefaultOutcomeAdapterRegistry())
	if registry.Len() != 5 {
		t.Fatalf("registry len = %d, want 5 with legacy adapters", registry.Len())
	}
}
