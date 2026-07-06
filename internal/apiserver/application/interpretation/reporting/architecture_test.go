package reporting

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
