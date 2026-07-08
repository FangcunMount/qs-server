package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentDocsDoNotReferenceRetiredCheckpointTablesAsCurrent(t *testing.T) {
	root := repoRoot(t)
	docsRoot := filepath.Join(root, "docs")
	allowedFiles := map[string]struct{}{
		"docs/02-业务模块/mechanism-oriented-migration.md": {},
		"docs/系统设计文档.md":                               {},
	}
	forbiddenTerms := []string{
		"analytics_projector_checkpoint",
		"`evaluation_run` 表",
	}

	err := filepath.WalkDir(docsRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if filepath.ToSlash(mustRel(t, root, path)) == "docs/_archive" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		if _, ok := allowedFiles[rel]; ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, forbidden := range forbiddenTerms {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s references retired checkpoint table %q as current documentation", rel, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs: %v", err)
	}
}
