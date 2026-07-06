package typology

import (
	"strings"
	"testing"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
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
