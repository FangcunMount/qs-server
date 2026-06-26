package typology

import (
	"testing"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
)

func TestReportAdapterRegistryBuildsByAdapterKey(t *testing.T) {
	registry := DefaultReportAdapterRegistry()

	spec := modeltypology.ReportSpec{Kind: modeltypology.ReportKindPersonalityType, AdapterKey: modeltypology.ReportAdapterMBTI}
	mapping := modeltypology.OutcomeMappingSpec{DetailAdapterKey: modeltypology.DetailAdapterMBTI}

	_, err := registry.build(spec, mapping, assessmentmodel.DecisionKindPoleComposition, evaluationresult.Outcome{})
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
}
