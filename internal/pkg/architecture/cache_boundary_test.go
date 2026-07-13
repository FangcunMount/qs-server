package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedCacheKernelDoesNotImportBusinessProcesses(t *testing.T) {
	root := repoRoot(t)
	forbidden := []string{
		"github.com/FangcunMount/qs-server/internal/apiserver",
		"github.com/FangcunMount/qs-server/internal/collection-server",
		"github.com/FangcunMount/qs-server/internal/worker",
	}
	walkGoFiles(t, filepath.Join(root, "internal", "pkg", "cache"), func(path, text string) {
		for _, importPath := range forbidden {
			if strings.Contains(text, importPath) {
				t.Fatalf("%s imports business process package %s", mustRel(t, root, path), importPath)
			}
		}
	})
}

func TestDomainPackagesDoNotImportCache(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		"internal/apiserver/domain",
		"internal/collection-server/domain",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		walkGoFiles(t, path, func(file, text string) {
			if strings.Contains(text, "github.com/FangcunMount/qs-server/internal/pkg/cache") ||
				strings.Contains(text, "github.com/FangcunMount/qs-server/internal/apiserver/cache") ||
				strings.Contains(text, "github.com/FangcunMount/qs-server/internal/collection-server/cache") {
				t.Fatalf("%s imports cache package", mustRel(t, root, file))
			}
		})
	}
}
