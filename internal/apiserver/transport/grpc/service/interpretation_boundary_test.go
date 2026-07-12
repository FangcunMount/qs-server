package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestReportRPCDelegatesDirectlyToInterpretationAutomationUseCase(t *testing.T) {
	data, err := os.ReadFile("interpretation_automation.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if !strings.Contains(source, "s.service.Generate(") {
		t.Fatal("report RPC must call the Interpretation automation use case")
	}
	for _, forbidden := range []string{"executeService.GenerateReport(", "executeService.GenerateReportFromOutcome("} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("report RPC must not route through Evaluation Service: %q", forbidden)
		}
	}
}

func TestInterpretationTraceIDReadsWorkerEventMetadata(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-event-id", "evt-42"))
	if got := interpretationTraceID(ctx); got != "evt-42" {
		t.Fatalf("trace id = %q, want evt-42", got)
	}
}
