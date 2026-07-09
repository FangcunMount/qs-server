package typology

import (
	"testing"

	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestDefaultAlgorithmAliases(t *testing.T) {
	aliases := DefaultAlgorithmAliases()
	if len(aliases) != 3 {
		t.Fatalf("len(aliases) = %d, want 3", len(aliases))
	}
}

func TestCategoryLabelForLegacyAlgorithms(t *testing.T) {
	if got := CategoryLabelFor(modelcatalog.AlgorithmSBTI); got != "SBTI" {
		t.Fatalf("CategoryLabelFor(sbti) = %q", got)
	}
	if got := CategoryLabelFor(modelcatalog.AlgorithmBigFive); got != "Big Five" {
		t.Fatalf("CategoryLabelFor(bigfive) = %q", got)
	}
}

func TestReportSpecForAlgorithmUsesLegacyDerivation(t *testing.T) {
	spec, _, _ := typologylegacy.ReportBuildContextFromAlgorithm(modelcatalog.AlgorithmBigFive)
	if spec.AdapterKey != modeltypology.ReportAdapterTraitProfile {
		t.Fatalf("adapter = %s, want trait_profile", spec.AdapterKey)
	}
	if spec.TemplateID != "bigfive" {
		t.Fatalf("template_id = %q, want bigfive", spec.TemplateID)
	}
}
