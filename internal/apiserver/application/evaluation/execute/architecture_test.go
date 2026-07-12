package execute

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

func TestEvaluatorContractsReturnDomainOutcomeExecution(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"runtime_resolver.go", "descriptor_executor.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "domainoutcome.Execution") {
			t.Fatalf("%s must expose domain outcome Execution as the evaluator result contract", path)
		}
	}
	data, err := os.ReadFile("../runtime/descriptor/contracts.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "domainoutcome.Execution") {
		t.Fatal("runtime descriptor contract must expose domain outcome Execution")
	}
}

func TestEngineServiceHasNoScoringWriterSuccessPath(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"WithScoringWriter", "scoringWriter", "outcome/scoring"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("service.go must commit successful evaluations only through EvaluationCommitter: %s", forbidden)
		}
	}
	if !strings.Contains(text, "evaluation committer is not configured") {
		t.Fatal("service.go must reject a successful evaluation when no EvaluationCommitter is configured")
	}
}

func TestEngineExecutesTheAlreadyResolvedDescriptor(t *testing.T) {
	data, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "ExecuteResolved(ctx, resolved") {
		t.Fatal("production execute path must reuse its resolved RuntimeDescriptor")
	}
	if strings.Contains(text, "runtimeResolver.Execute(ctx") {
		t.Fatal("production execute path resolves RuntimeDescriptor twice")
	}
}
