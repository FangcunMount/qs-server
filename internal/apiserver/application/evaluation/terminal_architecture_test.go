package evaluation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerminalEvaluationStructureHasNoRetiredPackages(t *testing.T) {
	for _, path := range []string{
		"assessment", "runquery", "consistency",
		filepath.Join("registry", "mechanisms", "typology", "legacy"),
		filepath.Join("..", "..", "domain", "evaluation", "pipeline"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("retired Evaluation package exists: %s", path)
		}
	}
	for _, path := range []string{"intake", "testee", "operator", "worker", "scheduler", "execute", "outcome", "runtime", "registry", "calculationadapter"} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Fatalf("required Evaluation package missing: %s", path)
		}
	}
}

func TestOperatorCommandsUseSharedAuthorizer(t *testing.T) {
	for _, path := range []string{filepath.Join("operator", "batch.go"), filepath.Join("operator", "service.go")} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "authorizer") {
			t.Fatalf("%s must use the package-private Operator authorizer", path)
		}
	}
}
