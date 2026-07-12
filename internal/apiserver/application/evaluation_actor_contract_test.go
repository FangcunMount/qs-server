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
			applicationFile: "internal/apiserver/application/journey/assessmentintake/service.go",
			applicationPort: "type Service interface",
			entryFile:       "internal/apiserver/transport/grpc/service/assessment_intake.go",
			entrySymbol:     "EnsureAssessment(",
			authorityFile:   "api/grpc/proto/evaluation/evaluation.proto",
			authoritySymbol: "service AssessmentIntakeService",
		},
		{
			actor:           "testee",
			applicationFile: "internal/apiserver/application/evaluation/testee/types.go",
			applicationPort: "type Service interface",
			entryFile:       "internal/apiserver/transport/grpc/service/evaluation.go",
			entrySymbol:     "GetMyAssessment(",
			authorityFile:   "internal/apiserver/application/evaluation/testee/service.go",
			authoritySymbol: "AuthorizeAssessment(",
		},
		{
			actor:           "backend operator",
			applicationFile: "internal/apiserver/application/evaluation/operator/types.go",
			applicationPort: "type QueryService interface",
			entryFile:       "internal/apiserver/transport/rest/handler/evaluation.go",
			entrySymbol:     "RequireProtectedScope(",
			authorityFile:   "internal/apiserver/application/evaluation/operator/batch.go",
			authoritySymbol: "type Actor struct",
		},
		{
			actor:           "scoring worker",
			applicationFile: "internal/apiserver/application/evaluation/worker/service.go",
			applicationPort: "type Service interface",
			entryFile:       "internal/apiserver/transport/grpc/service/evaluation_worker.go",
			entrySymbol:     "ExecuteEvaluation(",
			authorityFile:   "api/grpc/proto/evaluation/evaluation.proto",
			authoritySymbol: "service EvaluationWorkerService",
		},
		{
			actor:           "scheduler",
			applicationFile: "internal/apiserver/application/evaluation/scheduler/audit.go",
			applicationPort: "type Service interface",
			entryFile:       "internal/apiserver/runtime/scheduler/evaluation_consistency_reconcile.go",
			entrySymbol:     "AuditOnce(",
			authorityFile:   "internal/apiserver/application/evaluation/scheduler/audit.go",
			authoritySymbol: "read-only Evaluation maintenance",
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

func TestEvaluationTargetActorPackagesExist(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, name := range []string{"intake", "testee", "operator", "worker", "scheduler"} {
		path := filepath.Join(root, "internal/apiserver/application/evaluation", name)
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("target actor package %s is missing", name)
		}
	}
}

func TestEvaluationTransportCannotImportInternalMechanisms(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	got := collectCrossModuleImporters(t, root,
		[]string{"internal/apiserver/transport"},
		[]string{
			"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute",
			"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome",
			"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime",
			"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry",
			"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter",
		},
	)
	if len(got) != 0 {
		t.Fatalf("transport imports Evaluation mechanisms: %v", got)
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
		want := []string{}
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
		want := []string{}
		sort.Strings(want)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("answer-sheet orchestration debt changed\n got:\n%s\nwant:\n%s\nmove or shrink the allowlist; do not spread application behavior across transport", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	})
}

func TestEvaluationGRPCReportRPCsBelongToInterpretation(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "api/grpc/proto/interpretation/interpretation.proto"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if !strings.Contains(source, "service ParticipantReportService") {
		t.Fatal("participant report service is missing")
	}
	for _, rpc := range []string{"rpc GetAssessmentReport(", "rpc ListMyReports("} {
		if count := strings.Count(source, rpc); count != 1 {
			t.Fatalf("Interpretation participant RPC %q count = %d, want 1", rpc, count)
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
	for _, forbidden := range []string{"WorkerResultReader", "NewWorkerAssessmentResultReader("} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("retired worker result reader remains: %s", forbidden)
		}
	}
}

func TestRetiredEvaluationApplicationPackagesStayDeleted(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, name := range []string{"assessment", "runquery", "consistency"} {
		path := filepath.Join(root, "internal/apiserver/application/evaluation", name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("retired package still exists: %s", path)
		}
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
