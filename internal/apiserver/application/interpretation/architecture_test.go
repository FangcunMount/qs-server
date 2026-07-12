package interpretation_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestComposeDoesNotExposeInterpretationReportQueryToEvaluation(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "container", "compose", "ports.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"ReportIntegrationPorts", "ReportQueryService", "evaluationreadmodel.ReportReader", "ReportBuilderRegistry", "ReportDurableSaver", "ReportStateStore"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("compose ports must not leak Interpretation report capability %q into Evaluation", forbidden)
		}
	}
}

func TestInterpretationContainerOwnsOutcomeReportUseCase(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "container", "modules", "interpretation", "assemble.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"interpretationexecution.NewStarter", "interpretationexecution.NewInterpretationCommitter", "interpretationexecution.NewExecutor", "interpretationautomation.NewService", "interpretationparticipant.NewService", "interpretationadmin.NewService"} {
		if !strings.Contains(text, required) {
			t.Fatalf("interpretation assemble must own report capability %q", required)
		}
	}
	if strings.Contains(text, "execute.WithOutcomeReportService") {
		t.Fatal("interpretation assemble must not inject report generation back into evaluation")
	}
}

func TestExecutorDelegatesTerminalPersistenceToInterpretationCommitter(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "application", "interpretation", "automation", "execution", "executor.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"InterpretationCommitter", ".CommitSuccess(", ".CommitFailure("} {
		if !strings.Contains(text, required) {
			t.Fatalf("executor must delegate terminal persistence through InterpretationCommitter: %s", required)
		}
	}
	for _, forbidden := range []string{".reports.Insert(", ".generations.Save(", ".runs.Save(", ".stager.Stage("} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("executor must not persist terminal facts directly: %s", forbidden)
		}
	}
}

func TestInterpretationModuleOwnsReportCommitterWiring(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "container", "modules", "interpretation", "assemble.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"mongoEval.NewGenerationRepository", "mongoEval.NewRunRepository", "mongoEval.NewReportRepository"} {
		if !strings.Contains(text, required) {
			t.Fatalf("interpretation assemble must wire report lifecycle persistence %q", required)
		}
	}
	if strings.Contains(text, "NewTransactionalReportDurableSaver") {
		t.Fatal("interpretation assemble must not retain legacy InterpretReport durable saver")
	}
}

func TestArtifactRepositoryDoesNotSelectLatestReportByAssessment(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "domain", "interpretation", "report", "artifact_repository.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "FindLatestByAssessmentID") {
		t.Fatal("artifact repository must not select the current report; report_query_catalog owns that decision")
	}
}

func TestInterpretationRootContainsOnlyTerminalEventSurface(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	dir := filepath.Join(root, "internal", "apiserver", "domain", "interpretation")
	allowed := map[string]bool{"doc.go": true, "events.go": true, "events_outcome.go": true, "event_wire.go": true}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		if !allowed[entry.Name()] {
			t.Fatalf("interpretation root compatibility facade returned: %s", entry.Name())
		}
	}
}

func TestApplicationProjectionDoesNotInferLegacyPersistenceFields(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "application", "interpretation", "internal", "reportprojection", "mapper.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"domain/modelcatalog", "modelcatalog/binding", "row.TotalScore", "row.RiskLevel"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("application report projection must not infer legacy persistence field %q", forbidden)
		}
	}
}

func TestExecutionPackageOwnsReportEventStaging(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	reportingDir := filepath.Join(root, "internal", "apiserver", "application", "interpretation", "automation", "execution")
	found := false
	err := filepath.WalkDir(reportingDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "NewInterpretationReportGeneratedEvent") {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("interpretation/generation must own report generated event staging")
	}
}

func TestInterpretationCannotMutateAssessmentLifecycle(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	interpretationDir := filepath.Join(root, "internal", "apiserver", "application", "interpretation")
	forbidden := []string{".ApplyOutcome(", ".MarkAsFailed(", "assessment.Repository"}
	err := filepath.WalkDir(interpretationDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, token := range forbidden {
			if strings.Contains(string(data), token) {
				t.Fatalf("Interpretation lifecycle boundary violation in %s: %s", path, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutomationServiceCannotReevaluateOrWriteEvaluationFacts(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "application", "interpretation", "automation", "service.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "s.outcomes.FindByID") {
		t.Fatal("report retry must read the durable EvaluationOutcome by id")
	}
	if !strings.Contains(text, "interpretationinput.FromOutcomeRecord") {
		t.Fatal("production interpretation must build input directly from EvaluationOutcome")
	}
	if strings.Contains(text, "FromLegacyOutcome") {
		t.Fatal("production interpretation must not reconstruct legacy Outcome compatibility input")
	}
	for _, forbidden := range []string{
		"application/evaluation/execute",
		".Evaluate(",
		"assessment.Repository",
		"evaluationrun.Repository",
		"ScoreRepository",
		"ScoreProjector",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("automation service must not re-evaluate or write Evaluation facts: %s", forbidden)
		}
	}
}

// Keep the Batch I0 boundary guard broader than the outcome use case itself.
// Interpretation may read Evaluation facts through adapters during the
// transition, but it must never acquire Evaluation write ports or commands.
func TestInterpretationProductionCodeDoesNotAcquireEvaluationWriteCapabilities(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	paths := []string{
		filepath.Join(root, "internal", "apiserver", "domain", "interpretation"),
		filepath.Join(root, "internal", "apiserver", "application", "interpretation"),
		filepath.Join(root, "internal", "apiserver", "container", "modules", "interpretation"),
		filepath.Join(root, "internal", "apiserver", "infra", "mongo", "interpretation"),
	}
	forbidden := []string{
		"assessment.Repository",
		"evaluationrun.Repository",
		"ScoreRepository",
		"ScoreProjector",
		"application/evaluation/execute",
		"application/evaluation/runquery",
		"domain/evaluation/run",
		"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun\"",
		"infra/mongo/evaluation",
		".ApplyOutcome(",
		".ApplyScoringOutcome(",
		".ApplyScoringProjection(",
		".MarkAsFailed(",
	}

	for _, dir := range paths {
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for _, token := range forbidden {
				if strings.Contains(string(data), token) {
					t.Fatalf("Interpretation acquired Evaluation write capability in %s: %s", path, token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
