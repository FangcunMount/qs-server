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

func TestCachePackagesUseLoadguardInsteadOfSingleflightDirectly(t *testing.T) {
	root := repoRoot(t)
	matched := 0
	for _, rel := range []string{"internal/pkg/cache", "internal/apiserver/cache"} {
		walkGoFiles(t, filepath.Join(root, filepath.FromSlash(rel)), func(path, text string) {
			matched++
			if strings.Contains(text, "golang.org/x/sync/singleflight") {
				t.Fatalf("%s imports singleflight directly; use internal/pkg/loadguard", mustRel(t, root, path))
			}
		})
	}
	if matched == 0 {
		t.Fatal("cache singleflight boundary scan matched zero production files")
	}
}

func TestDomainPackagesDoNotImportCache(t *testing.T) {
	root := repoRoot(t)
	matched := 0
	for _, rel := range []string{
		"internal/apiserver/domain",
		"internal/collection-server/domain",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		walkGoFiles(t, path, func(file, text string) {
			matched++
			if strings.Contains(text, "github.com/FangcunMount/qs-server/internal/pkg/cache") ||
				strings.Contains(text, "github.com/FangcunMount/qs-server/internal/apiserver/cache") ||
				strings.Contains(text, "github.com/FangcunMount/qs-server/internal/collection-server/cache") ||
				strings.Contains(text, "github.com/FangcunMount/qs-server/internal/pkg/redisruntime") {
				t.Fatalf("%s imports cache package", mustRel(t, root, file))
			}
		})
	}
	if matched == 0 {
		t.Fatal("domain cache boundary scan matched zero production files")
	}
}
