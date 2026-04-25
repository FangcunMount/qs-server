package eventruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEventSystemDoesNotImportRemovedEventConfig(t *testing.T) {
	root := repoRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		if strings.Contains(text, removedEventConfigImportPath()) {
			t.Fatalf("%s must use eventcatalog/eventruntime instead of removed eventconfig", rel)
		}
		if strings.HasPrefix(rel, "internal/worker/integration/messaging/") &&
			strings.Contains(text, workerContainerImportPath()) {
			t.Fatalf("%s must depend on narrow messaging interfaces instead of worker/container", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
}

func removedEventConfigImportPath() string {
	return "github.com/FangcunMount/qs-server/internal/pkg/" + "eventconfig"
}

func workerContainerImportPath() string {
	return "github.com/FangcunMount/qs-server/internal/worker/" + "container"
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
