package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemovedLegacyPathsDoNotReturnToProductionCode(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		"internal/apiserver/infra/mysql/plan/Untitled",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			t.Fatalf("legacy stray file must not exist: %s", rel)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}

	forbiddenImports := []string{
		"github.com/FangcunMount/qs-server/internal/pkg/" + "eventconfig",
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/" + "outboxcodec",
		"github.com/FangcunMount/qs-server/internal/worker/" + "application",
		"github.com/FangcunMount/qs-server/internal/collection-server/interface/" + "restful",
	}

	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, forbidden := range forbiddenImports {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s imports removed or legacy path %s", mustRel(t, root, path), forbidden)
			}
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
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("repository root not found")
		}
		wd = parent
	}
}

func mustRel(t *testing.T, base, path string) string {
	t.Helper()
	rel, err := filepath.Rel(base, path)
	if err != nil {
		t.Fatalf("rel %s: %v", path, err)
	}
	return filepath.ToSlash(rel)
}
