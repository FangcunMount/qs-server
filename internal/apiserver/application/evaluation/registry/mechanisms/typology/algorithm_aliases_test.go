package typology

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
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
