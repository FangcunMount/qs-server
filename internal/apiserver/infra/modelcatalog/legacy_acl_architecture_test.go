package modelcatalog_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionCodeDoesNotImportLegacyACLPackage(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	forbidden := `github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy`
	allowPrefixes := []string{
		"internal/apiserver/domain/modelcatalog/legacy/",
		"internal/apiserver/domain/modelcatalog/export.go",
		"scripts/",
	}

	err := filepath.WalkDir(filepath.Join(root, "internal", "apiserver"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		for _, prefix := range allowPrefixes {
			if strings.HasPrefix(rel, prefix) {
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("%s imports legacy ACL package; runtime must use canonical published snapshots", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
