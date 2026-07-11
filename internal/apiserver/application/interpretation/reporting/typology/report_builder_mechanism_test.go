package typology_test

import (
	"testing"

	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	typologyreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/typology"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestTypologyReportBuilderRegistersDecisionMechanismKeys(t *testing.T) {
	t.Parallel()

	builder, err := typologyreporting.NewConfiguredReportBuilder()
	if err != nil {
		t.Fatalf("NewConfiguredReportBuilder: %v", err)
	}
	registry, err := interpretationreporting.NewReportBuilderRegistry(builder)
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry: %v", err)
	}
	for _, decision := range []modelcatalog.DecisionKind{
		modelcatalog.DecisionKindPoleComposition,
		modelcatalog.DecisionKindTraitProfile,
		modelcatalog.DecisionKindNearestPattern,
	} {
		_, err := registry.ResolveByMechanism(interpretationreporting.MechanismReportBuilderKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			DecisionKind:    decision,
			ReportType:      domainreport.ReportTypeStandard,
		})
		if err != nil {
			t.Fatalf("ResolveByMechanism(%s): %v", decision, err)
		}
	}
}
