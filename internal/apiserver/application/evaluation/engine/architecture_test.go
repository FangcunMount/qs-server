package engine

import (
	"os"
	"strings"
	"testing"
)

func TestEngineServiceDoesNotOwnPipelineAssemblyDependencies(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"buildPipeline",
		"scoreRepo",
		"reportRepo",
		"reportBuilder",
		"WithReportDurableSaver",
		"WithScaleFactorScorer",
		"WithInterpretEngine",
	} {
		if strings.Contains(text, token) {
			t.Fatalf("service.go contains %q; engine service should receive an explicit pipeline runner from composition root", token)
		}
	}
}
