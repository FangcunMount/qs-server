package materialize_test

import (
	"testing"

	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/materialize"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

func TestReportBuildersOwnEveryDefaultDescriptorPath(t *testing.T) {
	t.Parallel()

	paths := materialize.RegisteredPaths()
	builders, err := materialize.ReportBuilders(domainreport.NewDefaultReportBuilder(nil))
	if err != nil {
		t.Fatalf("ReportBuilders: %v", err)
	}
	if len(builders) != len(paths) {
		t.Fatalf("builder count = %d, want %d", len(builders), len(paths))
	}
	for i, want := range paths {
		got, err := interpretationreporting.ExecutionPathForReportBuilder(builders[i])
		if err != nil {
			t.Fatalf("builder path: %v", err)
		}
		if got != want {
			t.Fatalf("builder path[%d] = %s, want %s", i, got, want)
		}
	}
}
