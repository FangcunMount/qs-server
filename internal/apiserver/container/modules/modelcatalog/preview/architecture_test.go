package preview

import (
	"os"
	"strings"
	"testing"
)

func TestPreviewRemainsIndependentInProcessComposition(t *testing.T) {
	data, err := os.ReadFile("previewer.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, required := range []string{"executor.Execute(", "rendering.NewTypologyBuilder().Build("} {
		if !strings.Contains(source, required) {
			t.Fatalf("preview composition must contain %q", required)
		}
	}
	for _, forbidden := range []string{"OutcomeReportService", "GenerateByOutcomeID", "ReportDurableSaver", "application/interpretation"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("preview must not depend on production report orchestration %q", forbidden)
		}
	}
}
