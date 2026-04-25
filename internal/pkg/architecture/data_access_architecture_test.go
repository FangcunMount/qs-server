package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataAccessPackagesDoNotDependOnTransportImplementations(t *testing.T) {
	root := repoRoot(t)
	for _, relRoot := range []string{
		"internal/apiserver/infra/mysql",
		"internal/apiserver/infra/mongo",
		"internal/pkg/database",
		"internal/pkg/mongodb",
		"internal/pkg/migration",
	} {
		walkGoFiles(t, filepath.Join(root, relRoot), func(path string, text string) {
			for _, forbidden := range []string{
				"github.com/FangcunMount/qs-server/internal/apiserver/transport/",
				"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful",
				"github.com/FangcunMount/qs-server/internal/collection-server/transport/",
			} {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s must not import transport/interface implementation path %s", mustRel(t, root, path), forbidden)
				}
			}
		})
	}
}

func TestDomainPackagesDoNotDependOnInfrastructure(t *testing.T) {
	root := repoRoot(t)
	walkGoFiles(t, filepath.Join(root, "internal/apiserver/domain"), func(path string, text string) {
		for _, forbidden := range []string{
			"github.com/FangcunMount/qs-server/internal/apiserver/infra/",
			"github.com/FangcunMount/qs-server/internal/pkg/database",
			"github.com/FangcunMount/qs-server/internal/pkg/mongodb",
			"github.com/FangcunMount/qs-server/internal/pkg/migration",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s must keep domain model independent from data access infrastructure %s", mustRel(t, root, path), forbidden)
			}
		}
	})
}

func walkGoFiles(t *testing.T, root string, visit func(path string, text string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
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
		visit(path, string(data))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}
