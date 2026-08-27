package aiexplanation_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAILifecyclePackagesDoNotDependOnEvaluationOrProviderInfrastructure(t *testing.T) {
	t.Parallel()

	root := packageRoot(t)
	forbidden := []string{
		"application/evaluation",
		"domain/evaluation",
		"infra/mongo/evaluation",
		"infra/aiexplanation/provider",
		"openai-go",
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, token := range forbidden {
			if strings.Contains(string(data), token) {
				t.Fatalf("AI explanation domain depends on forbidden boundary %q in %s", token, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func packageRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve AI explanation package root")
	}
	return filepath.Dir(file)
}
