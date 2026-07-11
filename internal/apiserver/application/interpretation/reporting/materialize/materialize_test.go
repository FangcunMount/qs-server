package materialize_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/materialize"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

func TestReportBuildersOwnEveryDefaultDescriptorPath(t *testing.T) {
	t.Parallel()

	descs := registry.DefaultEvaluationDescriptors()
	builders, err := materialize.ReportBuilders(descs, domainreport.NewDefaultInterpretReportBuilder(nil))
	if err != nil {
		t.Fatalf("ReportBuilders: %v", err)
	}
	if len(builders) != len(descs) {
		t.Fatalf("builder count = %d, want %d", len(builders), len(descs))
	}
	for i, desc := range descs {
		want, err := evaldomain.ExecutionPathForDescriptor(desc)
		if err != nil {
			t.Fatalf("descriptor path: %v", err)
		}
		got, err := interpretationreporting.ExecutionPathForReportBuilder(builders[i])
		if err != nil {
			t.Fatalf("builder path: %v", err)
		}
		if got != want {
			t.Fatalf("builder path[%d] = %s, want %s", i, got, want)
		}
	}
}
