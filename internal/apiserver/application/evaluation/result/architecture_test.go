package result

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMongoReportRepositoryDoesNotExposeDurableSaverEntry(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "infra", "mongo", "evaluation", "repo.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "func (r *ReportRepository) SaveReportDurably") {
		t.Fatalf("%s exposes SaveReportDurably; production wiring must use application ReportDurableSaver only", filepath.ToSlash(mustRel(t, root, path)))
	}
}

func TestResultLayerOwnsSingleReportDurableSaverPort(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	dir := filepath.Join(root, "internal", "apiserver", "application", "evaluation", "result")
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		if strings.Contains(text, "type ReportDurableSaver interface") {
			return nil
		}
		if strings.Contains(text, "SaveReportDurably(ctx") && !strings.Contains(path, "report_durable_saver.go") && !strings.Contains(path, "writer.go") {
			t.Fatalf("%s defines SaveReportDurably outside the durable saver port; keep durable report writes behind ReportDurableSaver", filepath.ToSlash(mustRel(t, root, path)))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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

func mustRel(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}
