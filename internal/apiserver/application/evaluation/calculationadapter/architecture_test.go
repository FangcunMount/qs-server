package calculationadapter_test

import (
	"os"
	"strings"
	"testing"
)

func TestCalculationAdapterSharesGenericOutcomeBridge(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	required := map[string]struct{}{
		"outcome.go":         {},
		"score.go":           {},
		"score_node.go":      {},
		"scoring_outcome.go": {},
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		delete(required, entry.Name())
	}
	for missing := range required {
		t.Fatalf("missing shared calculationadapter file %s; norming/scoring should reuse outcome.go and score_node.go", missing)
	}
}
