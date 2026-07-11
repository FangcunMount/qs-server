package interpretation_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestComposeReportPortsExposeOnlyReadModelToEvaluation(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "container", "compose", "ports.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "evaluationreadmodel.ReportReader") {
		t.Fatal("compose report ports must expose the report reader")
	}
	for _, forbidden := range []string{"ReportBuilderRegistry", "ReportDurableSaver", "ReportStateStore"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("compose report ports must not leak Interpretation write capability %q", forbidden)
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
	if !strings.Contains(text, "interpretationreporting.NewGenerator") || !strings.Contains(text, "interpretationapp.NewOutcomeReportService") {
		t.Fatal("interpretation assemble must own outcome report generation use case")
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
