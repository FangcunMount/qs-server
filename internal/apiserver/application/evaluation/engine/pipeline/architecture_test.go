package pipeline

import (
	"os"
	"strings"
	"testing"
)

func TestPipelineHandlersDoNotOwnPersistenceOrNotificationDetails(t *testing.T) {
	t.Parallel()

	assertFileDoesNotContain(t, "risk_level.go", []string{
		"SaveScoresWithContext",
		"ScoreRepository",
	})
	assertFileDoesNotContain(t, "interpretation.go", []string{
		"SaveReportDurably",
		"ApplyEvaluation(",
		"NewReportGeneratedEvent",
		"NewFootprintReportGeneratedEvent",
		"ReportRepository",
		"ReportBuilder",
		"repositoryAssessmentResultWriter{}",
		"durableInterpretReportWriter{}",
		"ensureFinalizer",
		"ensureAssessmentWriter",
		"ensureReportWriter",
	})
	assertFileDoesNotContain(t, "waiter_notify.go", []string{
		"StatusSummary",
		"GetWaiterCount",
	})
}

func TestPipelineHandlersDoNotHoldRepositories(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"factor_score.go",
		"risk_level.go",
		"interpretation.go",
		"waiter_notify.go",
	} {
		assertFileDoesNotContain(t, path, []string{
			"Repository",
			"SaveScoresWithContext",
			"SaveReportDurably",
		})
	}
}

func TestInterpretationHandlerFileDoesNotOwnGeneratorDetails(t *testing.T) {
	t.Parallel()

	assertFileDoesNotContain(t, "interpretation.go", []string{
		"interpretFactorWithRules",
		"buildInterpretConfig",
		"tryInterpretWithTotalScoreRule",
		"logRuleMatch",
	})
}

func assertFileDoesNotContain(t *testing.T, path string, forbidden []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range forbidden {
		if strings.Contains(text, token) {
			t.Fatalf("%s contains %q; pipeline handlers should delegate side effects to writer/notifier seams", path, token)
		}
	}
}
