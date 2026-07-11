package service

import (
	"os"
	"strings"
	"testing"
)

func TestReportRPCDelegatesDirectlyToInterpretationOutcomeUseCase(t *testing.T) {
	data, err := os.ReadFile("internal_assessment_flow.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if !strings.Contains(source, "outcomeReportService.GenerateByOutcomeID(") {
		t.Fatal("report RPC must call the Interpretation outcome use case")
	}
	for _, forbidden := range []string{"executeService.GenerateReport(", "executeService.GenerateReportFromOutcome("} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("report RPC must not route through Evaluation Service: %q", forbidden)
		}
	}
}
