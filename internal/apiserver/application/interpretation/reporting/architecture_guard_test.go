package reporting_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReportingUsesMechanismBuilderNames(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	reportingRoot := filepath.Join(root, "internal", "apiserver", "application", "interpretation", "reporting")
	forbidden := []string{
		"NewMBTIReportBuilder",
		"NewSBTIReportBuilder",
		"NewScaleReportBuilder(",
		"NewScaleScoreProjector(",
		"NewBehavioralRatingReportBuilder(",
		"NewBehavioralRatingScoreProjector(",
		"NewCognitiveReportBuilder(",
		"NewCognitiveScoreProjector(",
		"ScoreRepository",
		"ScoreProjector",
		"SaveProjectionFromOutcome",
	}
	err := filepath.WalkDir(reportingRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Fatalf("%s contains deprecated token %q", filepath.Base(path), token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReportingDoesNotReintroduceLegacyReportAliasesFile(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "application", "interpretation", "reporting", "legacy_report_aliases.go")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("legacy_report_aliases.go must not exist; use mechanism-oriented builder names")
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
