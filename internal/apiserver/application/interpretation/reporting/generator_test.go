package reporting

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

func TestGeneratorEmitsReportEventsWithoutAssessmentInterpreted(t *testing.T) {
	order := make([]string, 0)
	a := submittedScaleAssessment(t)
	outcome := scaleOutcomeForWriterTest(a)
	builders, err := NewReportBuilderRegistry(scaleReportBuilderStub(&order, domainreport.NewInterpretReport(domainreport.ID(a.ID()), "Scale", "S-1", 7, domainreport.RiskLevelLow, "ok", nil, nil, nil), nil))
	if err != nil {
		t.Fatal(err)
	}
	generator, err := NewGenerator(builders)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := generator.Generate(context.Background(), outcome)
	if err != nil {
		t.Fatal(err)
	}
	for _, evt := range generation.Events {
		if evt.EventType() == assessment.EventTypeInterpretedOutcome {
			t.Fatalf("independent report generation emitted %s", evt.EventType())
		}
	}
	if len(generation.Events) != 2 {
		t.Fatalf("report events = %d, want report.generated + footprint", len(generation.Events))
	}
}
