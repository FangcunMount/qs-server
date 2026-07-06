package typology

import (
	"strings"
	"testing"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func TestReportAdapterRegistryBuildsByAdapterKey(t *testing.T) {
	registry := DefaultReportAdapterRegistry()

	spec := modeltypology.ReportSpec{Kind: modeltypology.ReportKindPersonalityType, AdapterKey: modeltypology.ReportAdapterMBTI}
	mapping := modeltypology.OutcomeMappingSpec{DetailAdapterKey: modeltypology.DetailAdapterMBTI}

	_, err := registry.build(spec, mapping, modelcatalog.DecisionKindPoleComposition, evaluationresult.Outcome{})
	if err == nil {
		t.Fatal("expected error without assessment")
	}
	if err.Error() != "assessment is required" {
		t.Fatalf("build error = %v, want assessment required", err)
	}
}

func TestReportAdapterRegistryRejectsTemplateKind(t *testing.T) {
	registry := DefaultReportAdapterRegistry()
	_, err := registry.build(
		modeltypology.ReportSpec{Kind: modeltypology.ReportKindTemplate, TemplateID: "x"},
		modeltypology.OutcomeMappingSpec{},
		"",
		evaluationresult.Outcome{},
	)
	if err == nil {
		t.Fatal("expected template kind error")
	}
	if !strings.Contains(err.Error(), "report adapter key is required") {
		t.Fatalf("build error = %v, want report adapter key required", err)
	}
}

func TestReportAdapterRegistryRejectsUnknownAdapter(t *testing.T) {
	registry := DefaultReportAdapterRegistry()
	_, err := registry.build(
		modeltypology.ReportSpec{Kind: modeltypology.ReportKindPersonalityType, AdapterKey: modeltypology.ReportAdapterKey("custom_unknown")},
		modeltypology.OutcomeMappingSpec{},
		modelcatalog.DecisionKindPoleComposition,
		evaluationresult.Outcome{},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported report adapter key") {
		t.Fatalf("build error = %v, want unsupported report adapter key", err)
	}
}
