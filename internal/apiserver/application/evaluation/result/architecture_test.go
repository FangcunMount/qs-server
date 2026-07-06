package result

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var allowedResultImplementations = map[string]bool{
	"scoring_snapshot_store.go": true,
}

var delegateOnlyFiles = map[string]bool{
	"doc.go":                           true,
	"types.go":                         true,
	"writer.go":                        true,
	"registry.go":                      true,
	"events.go":                        true,
	"report_durable_saver.go":          true,
	"interpretation_writer.go":         true,
	"interpretation_writer_factory.go": true,
	"report_projection.go":             true,
	"report_strategy.go":               true,
	"scale.go":                         true,
	"waiter.go":                        true,
}

func TestResultPackageMarkedDeprecated(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join(packageDir(t), "doc.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Deprecated") {
		t.Fatal("evaluation/result doc.go must mark package as deprecated compatibility facade")
	}
}

func TestResultPackageKeepsReportingLogicInInterpretationReporting(t *testing.T) {
	t.Parallel()

	dir := packageDir(t)
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		base := filepath.Base(path)
		if allowedResultImplementations[base] {
			return nil
		}
		if !delegateOnlyFiles[base] {
			t.Fatalf("%s is not classified; new logic must live in interpretation/reporting", base)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		if !strings.Contains(text, "interpretation/reporting") && base != "types.go" {
			t.Fatalf("%s must delegate to interpretation/reporting", base)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMongoReportRepositoryDoesNotExposeDurableSaverEntry(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "infra", "mongo", "interpretation", "repo.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "func (r *ReportRepository) SaveReportDurably") {
		t.Fatalf("%s exposes SaveReportDurably; production wiring must use application ReportDurableSaver only", filepath.ToSlash(mustRel(t, root, path)))
	}
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}
	return filepath.Dir(file)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir := packageDir(t)
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

func mustRel(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}
