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
	for _, required := range []string{"interpretationreporting.NewGenerator", "interpretationapp.NewOutcomeReportService", "interpretationapp.NewReportQueryService"} {
		if !strings.Contains(text, required) {
			t.Fatalf("interpretation assemble must own report capability %q", required)
		}
	}
	if strings.Contains(text, "execute.WithOutcomeReportService") {
		t.Fatal("interpretation assemble must not inject report generation back into evaluation")
	}
}

func TestInterpretationModuleOwnsReportDurableSaverWiring(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "container", "modules", "interpretation", "assemble.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "interpretationreporting.NewTransactionalReportDurableSaver") {
		t.Fatalf("interpretation assemble must wire transactional report durable saver from interpretation/reporting")
	}
	if strings.Contains(text, "evaluation/result.NewTransactionalReportDurableSaver") {
		t.Fatal("interpretation assemble must not wire report durable saver from evaluation/result")
	}
}

func TestReportingPackageOwnsEventStaging(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	reportingDir := filepath.Join(root, "internal", "apiserver", "application", "interpretation", "reporting")
	found := false
	err := filepath.WalkDir(reportingDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "buildReportGeneratedOutcomeEvent") {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("interpretation/reporting must own report generated event staging")
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

func TestOutcomeReportServiceCannotReevaluateOrWriteEvaluationFacts(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "application", "interpretation", "service.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "s.outcomes.FindByID") {
		t.Fatal("report retry must read the durable EvaluationOutcome by id")
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
			t.Fatalf("OutcomeReportService must not re-evaluate or write Evaluation facts: %s", forbidden)
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
		"port/evaluationrun",
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
