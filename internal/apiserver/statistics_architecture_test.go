package apiserver

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStatisticsRuntimeDoesNotQueryInterpretedAssessmentStatus(t *testing.T) {
	t.Parallel()

	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve statistics architecture test path")
	}
	root := filepath.Dir(current)
	for _, dir := range []string{
		filepath.Join(root, "application", "statistics"),
		filepath.Join(root, "domain", "statistics"),
		filepath.Join(root, "infra", "mysql", "statistics"),
		filepath.Join(root, "cache", "statistics"),
		filepath.Join(root, "container", "modules", "statistics"),
	} {
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(body), `"interpreted"`) {
				t.Fatalf("%s contains retired assessment status interpreted", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
