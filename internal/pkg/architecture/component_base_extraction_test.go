package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractedProcessRuntimeIsImportedFromComponentBase(t *testing.T) {
	root := repoRoot(t)
	forbidden := "github.com/FangcunMount/qs-server/internal/pkg/processruntime"
	required := "github.com/FangcunMount/component-base/pkg/processruntime"

	foundRequired := false
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if importPath == forbidden {
				t.Fatalf("%s imports extracted qs processruntime; use component-base processruntime", mustRel(t, root, path))
			}
			if importPath == required {
				foundRequired = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
	if !foundRequired {
		t.Fatalf("expected at least one internal process package to import %s", required)
	}
}

func TestExtractedProcessRuntimePackageHasNoLocalGoFiles(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "internal", "pkg", "processruntime")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("read processruntime dir: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" {
			t.Fatalf("internal/pkg/processruntime/%s remains after extraction; use component-base processruntime", entry.Name())
		}
	}
}

func TestSharedTransactionEventAndOutboxPackagesUseApprovedOwners(t *testing.T) {
	root := repoRoot(t)
	requiredByFile := map[string]string{
		"internal/pkg/database/mysql/uow.go":         "github.com/FangcunMount/component-base/pkg/uow/gorm",
		"internal/pkg/eventing/runtime/publisher.go": "github.com/FangcunMount/reliable-messaging/wire/domain",
		"internal/apiserver/port/outbox/outbox.go":   "github.com/FangcunMount/reliable-messaging/outbox",
	}

	for rel, required := range requiredByFile {
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, rel), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		found := false
		for _, imported := range file.Imports {
			if strings.Trim(imported.Path.Value, `"`) == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s must import approved shared package %s", rel, required)
		}
	}

	// The host status API must not inherit the legacy component-base type even
	// while the old Outbox core remains available for the rollback window.
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "internal/apiserver/port/outbox/outbox.go"), nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse outbox port: %v", err)
	}
	for _, imported := range file.Imports {
		if strings.Trim(imported.Path.Value, `"`) == "github.com/FangcunMount/component-base/pkg/outbox" {
			t.Fatal("host outbox status port must not import component-base/pkg/outbox")
		}
	}
}

// These imports remain only for the old Outbox rollback and message bridge.
// Removing an entry is allowed; adding a dependency on the retiring component-base
// message packages, including RabbitMQ, is not.
func TestComponentBaseMessageImportsAreRetired(t *testing.T) {
	root := repoRoot(t)
	const base = "github.com/FangcunMount/component-base/pkg/"
	messagePackages := []string{
		"event", "eventcatalog", "eventcodec", "eventmessaging", "messaging", "outbox", "outboxcore",
	}
	allowed := map[string]map[string]bool{}

	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			for _, pkg := range messagePackages {
				prefix := base + pkg
				if importPath != prefix && !strings.HasPrefix(importPath, prefix+"/") {
					continue
				}
				if !allowed[rel][importPath] {
					t.Errorf("%s adds retiring component-base message import %s", rel, importPath)
				}
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
}
