package typology

import (
	"testing"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestDefaultReportAdapterRegistryContainsConfiguredAdapters(t *testing.T) {
	registry := DefaultReportAdapterRegistry()
	for _, key := range []modeltypology.ReportAdapterKey{
		modeltypology.ReportAdapterPersonalityType,
		modeltypology.ReportAdapterTraitProfile,
		modeltypology.ReportAdapterMBTI,
		modeltypology.ReportAdapterSBTI,
		modeltypology.ReportAdapterBigFive,
	} {
		if !registry.Supports(key) {
			t.Fatalf("adapter %s is not registered", key)
		}
	}
}

func TestReportAdapterRegistryRegisterReturnsIndependentCopy(t *testing.T) {
	base := ReportAdapterRegistry{adapters: map[modeltypology.ReportAdapterKey]struct{}{modeltypology.ReportAdapterPersonalityType: {}}}
	custom := modeltypology.ReportAdapterKey("custom")
	next := base.Register(custom)
	if base.Supports(custom) || !next.Supports(custom) {
		t.Fatalf("registry copy semantics are invalid")
	}
}
