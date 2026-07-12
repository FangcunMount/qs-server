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

func TestTerminalEvaluationSourcesDoNotRestoreRetiredAPIs(t *testing.T) {
	checks := map[string][]string{
		filepath.Join("..", "..", "domain", "evaluation", "routing", "resolve.go"): {"ExecutionRoutingFromRoute", "ExecutionPathForFamily"},
		filepath.Join("runtime", "descriptor", "contracts.go"):                     {"func PersonalityTypologyIdentity", "func ExecutionIdentityFromLegacyKind"},
		filepath.Join("runtime", "descriptor", "registry.go"):                      {"ExecutionPathForFamily"},
		filepath.Join("scheduler", "audit.go"):                                     {"gorm.io/gorm", "gorm.ErrRecordNotFound"},
	}
	for path, forbidden := range checks {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(data), token) {
				t.Fatalf("%s restored retired Evaluation API %q", path, token)
			}
		}
	}
	if _, err := os.Stat(filepath.Join("..", "..", "domain", "evaluation", "input", "ref.go")); !os.IsNotExist(err) {
		t.Fatal("retired evaluation input SnapshotRef exists")
	}
}

func TestEvaluationModuleDoesNotExposeMechanismOrWorkbenchReaders(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "container", "modules", "evaluation", "assemble.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"WorkerExecutionService", "LatestRiskReader evaluationreadmodel", "AssessmentReader evaluationreadmodel"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("Evaluation Module exposes retired field %q", forbidden)
		}
	}
}
