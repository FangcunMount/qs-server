package legacy_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
)

func TestDefaultAlgorithmAliases(t *testing.T) {
	aliases := legacy.DefaultAlgorithmAliases()
	if len(aliases) != 3 {
		t.Fatalf("aliases = %#v", aliases)
	}
}

func TestReportSpecForAlgorithmDelegatesToDomainLegacy(t *testing.T) {
	spec := legacy.ReportSpecForAlgorithm(assessmentmodel.AlgorithmBigFive)
	if spec.Kind != modeltypology.ReportKindTraitProfile {
		t.Fatalf("kind = %s", spec.Kind)
	}
}
