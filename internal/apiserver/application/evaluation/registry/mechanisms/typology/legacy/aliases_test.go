package legacy_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func TestDefaultAlgorithmAliases(t *testing.T) {
	aliases := legacy.DefaultAlgorithmAliases()
	want := []modelcatalog.Algorithm{
		modelcatalog.AlgorithmMBTI,
		modelcatalog.AlgorithmSBTI,
		modelcatalog.AlgorithmBigFive,
	}
	if len(aliases) != len(want) {
		t.Fatalf("aliases = %#v, want %#v", aliases, want)
	}
	for i := range want {
		if aliases[i] != want[i] {
			t.Fatalf("aliases[%d] = %s, want %s", i, aliases[i], want[i])
		}
	}
}

func TestDefaultAlgorithmAliasesMatchEvaluatorLegacyKeys(t *testing.T) {
	aliases := legacy.DefaultAlgorithmAliases()
	keys := evaluation.PersonalityTypologyLegacyKeys()
	if len(aliases) != len(keys) {
		t.Fatalf("aliases = %#v, legacy keys = %#v", aliases, keys)
	}
	for i, algorithm := range aliases {
		if keys[i].Algorithm != algorithm {
			t.Fatalf("aliases[%d] = %s, key algorithm = %s", i, algorithm, keys[i].Algorithm)
		}
	}
}

func TestReportSpecForAlgorithmDelegatesToDomainLegacy(t *testing.T) {
	spec := legacy.ReportSpecForAlgorithm(modelcatalog.AlgorithmBigFive)
	if spec.Kind != modeltypology.ReportKindTraitProfile {
		t.Fatalf("kind = %s", spec.Kind)
	}
}
