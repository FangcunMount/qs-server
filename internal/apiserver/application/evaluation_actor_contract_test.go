package application_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestEvaluationActorEntryContracts keeps the actor-facing application ports
// tied to their real transport/runtime entrypoints. Internal Evaluation
// mechanisms are intentionally absent: they are implementation details, not
// independent actors.
func TestEvaluationActorEntryContracts(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	contracts := []struct {
		actor           string
		applicationFile string
		applicationPort string
		entryFile       string
		entrySymbol     string
		authorityFile   string
		authoritySymbol string
	}{
		{
			actor:           "answer-sheet orchestrator",
			applicationFile: "internal/apiserver/application/evaluation/assessment/interface.go",
			applicationPort: "type AnswerSheetAssessmentIntakeService interface",
			entryFile:       "internal/apiserver/transport/grpc/service/internal.go",
			entrySymbol:     "CreateAssessmentFromAnswerSheet(",
			authorityFile:   "api/grpc/proto/internalapi/internal.proto",
			authoritySymbol: "service InternalService",
		},
		{
			actor:           "testee",
			applicationFile: "internal/apiserver/application/evaluation/assessment/interface.go",
			applicationPort: "type TesteeAssessmentQueryService interface",
			entryFile:       "internal/apiserver/transport/grpc/service/evaluation.go",
			entrySymbol:     "GetMyAssessment(",
			authorityFile:   "internal/apiserver/application/evaluation/assessment/submission_getter.go",
			authoritySymbol: "a.TesteeID().Uint64() != testeeID",
		},
		{
			actor:           "backend operator",
			applicationFile: "internal/apiserver/application/evaluation/assessment/interface.go",
			applicationPort: "type AssessmentProtectedQueryService interface",
			entryFile:       "internal/apiserver/transport/rest/handler/evaluation.go",
			entrySymbol:     "RequireProtectedScope(",
			authorityFile:   "internal/apiserver/application/evaluation/assessment/protected_query_service.go",
			authoritySymbol: "ProtectedQueryScope",
		},
		{
			actor:           "scoring worker",
			applicationFile: "internal/apiserver/application/evaluation/execute/interface.go",
			applicationPort: "type WorkerExecutionService interface",
			entryFile:       "internal/apiserver/transport/grpc/service/internal.go",
			entrySymbol:     "EvaluateAssessment(",
			authorityFile:   "api/grpc/proto/internalapi/internal.proto",
			authoritySymbol: "service InternalService",
		},
		{
			actor:           "scheduler",
			applicationFile: "internal/apiserver/application/evaluation/consistency/reconcile_service.go",
			applicationPort: "type Service interface",
			entryFile:       "internal/apiserver/runtime/scheduler/evaluation_consistency_reconcile.go",
			entrySymbol:     "ReconcileOnce(",
			authorityFile:   "internal/apiserver/application/evaluation/consistency/reconcile_service.go",
			authoritySymbol: "background schedulers",
		},
	}

	for _, contract := range contracts {
		contract := contract
		t.Run(contract.actor, func(t *testing.T) {
			assertSourceContains(t, root, contract.applicationFile, contract.applicationPort)
			assertSourceContains(t, root, contract.entryFile, contract.entrySymbol)
			assertSourceContains(t, root, contract.authorityFile, contract.authoritySymbol)
		})
	}
}

// TestEvaluationTransportBusinessDebtDoesNotSpread is a ratchet, not the
// target architecture. The listed files contain known orchestration that a
// later batch will move behind actor/Journey application services. H0 freezes
// the debt so no new transport file can acquire the same responsibilities.
func TestEvaluationTransportBusinessDebtDoesNotSpread(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	t.Run("direct evaluation read-model use", func(t *testing.T) {
		got := collectCrossModuleImporters(t, root,
			[]string{"internal/apiserver/transport/grpc/service"},
			[]string{"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"},
		)
		want := []string{"internal/apiserver/transport/grpc/service/evaluation.go"}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("direct Evaluation read-model debt changed\n got:\n%s\nwant:\n%s\nmove or shrink the allowlist; do not add transport readers", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	})

	t.Run("cross-module answer-sheet orchestration", func(t *testing.T) {
		got := collectSourceTokenFiles(t, root,
			[]string{"internal/apiserver/transport/grpc/service"},
			[]string{
				"CalculateAndSave(",
				"ResolveAssessmentBinding(",
				"ResolveTaskByIDForAssessment(",
				"completeMatchedTask(",
				"SetQueued(",
				"SubmitForEvaluation(",
			},
		)
		want := []string{
			"internal/apiserver/transport/grpc/service/internal.go",
			"internal/apiserver/transport/grpc/service/internal_assessment_flow.go",
		}
		sort.Strings(want)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("answer-sheet orchestration debt changed\n got:\n%s\nwant:\n%s\nmove or shrink the allowlist; do not spread application behavior across transport", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	})
}

// EvaluationService still exposes two report-viewer compatibility RPCs. They
// may only shrink until Interpretation/Journey owns the replacement service.
func TestEvaluationGRPCReportCompatibilityDebtDoesNotSpread(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "api/grpc/proto/evaluation/evaluation.proto"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, rpc := range []string{"rpc GetAssessmentReport(", "rpc ListMyReports("} {
		if count := strings.Count(source, rpc); count != 1 {
			t.Fatalf("Evaluation report compatibility RPC %q count = %d, want 1; remove the ratchet entry when the RPC migrates", rpc, count)
		}
	}
	for _, forbidden := range []string{"rpc WaitReport(", "rpc RetryReport(", "rpc GenerateReport("} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("EvaluationService gained report-viewer behavior %q; report APIs belong to Interpretation/Journey", forbidden)
		}
	}
}

func TestEvaluationWorkerDoesNotReuseOperatorQueryService(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal/apiserver/container/modules/evaluation/assemble.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "WorkerResultReader = m.OperatorQueryService") {
		t.Fatal("Worker result reader must not reuse the backend-operator query service")
	}
	if !strings.Contains(string(data), "NewWorkerAssessmentResultReader(") {
		t.Fatal("Evaluation assembly must wire the dedicated Worker result reader")
	}
}

func assertSourceContains(t *testing.T, root, rel, token string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), token) {
		t.Fatalf("%s is missing actor contract token %q", rel, token)
	}
}
