package reporting_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type stubMechanismBuilder struct {
	reporting.FactorScoringReportBuilder
	key reporting.MechanismReportBuilderKey
}

func (b stubMechanismBuilder) MechanismKey() reporting.MechanismReportBuilderKey {
	return b.key
}

func TestResolveByMechanismFallsBackFromAlgorithmToFamily(t *testing.T) {
	familyKey := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
	}
	registry, err := reporting.NewReportBuilderRegistry(
		stubMechanismBuilder{key: familyKey},
	)
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry: %v", err)
	}
	specific := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
		Algorithm:       modelcatalog.AlgorithmScaleDefault,
		ProductChannel:  modelcatalog.ProductChannelMedicalScale,
	}
	builder, err := registry.ResolveByMechanism(specific)
	if err != nil {
		t.Fatalf("ResolveByMechanism: %v", err)
	}
	if builder == nil {
		t.Fatal("builder is nil")
	}
}
