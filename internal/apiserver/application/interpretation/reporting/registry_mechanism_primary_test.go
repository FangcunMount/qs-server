package reporting

import (
	"testing"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestResolveUsesMechanismForNormProfileBuilder(t *testing.T) {
	t.Parallel()
	registry, err := NewReportBuilderRegistry(NewNormProfileReportBuilder(nil))
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	builder, err := registry.ResolveByMechanism(MechanismReportBuilderKey{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm, DecisionKind: modelcatalog.DecisionKindNormLookup, ReportType: domainReport.ReportTypeStandard})
	if err != nil {
		t.Fatal(err)
	}
	keyed, ok := builder.(MechanismKeyedReportBuilder)
	if !ok || keyed.MechanismKey().AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorNorm {
		t.Fatalf("builder mechanism = %#v", builder)
	}
}
