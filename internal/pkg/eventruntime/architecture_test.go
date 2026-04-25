package eventruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEventRuntimeProductionCodeDoesNotUseGlobalEventConfig(t *testing.T) {
	root := repoRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		if strings.HasPrefix(rel, "internal/pkg/eventconfig/") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		if strings.Contains(text, "eventconfig.Global(") || strings.Contains(text, "eventconfig.Initialize(") {
			t.Fatalf("%s must not use eventconfig global registry in production code", rel)
		}
		if (strings.Contains(rel, "/domain/") || strings.Contains(rel, "/application/")) &&
			strings.Contains(text, `"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"`) {
			t.Fatalf("%s must use eventcatalog/eventruntime instead of eventconfig", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "../../.."))
}

func mustRel(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("rel %q: %v", path, err)
	}
	return rel
}
