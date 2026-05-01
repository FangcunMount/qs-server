package domain_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDomainDoesNotImportInfrastructureDrivers(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	domainRoot := filepath.Join(root, "internal", "apiserver", "domain")
	forbiddenImports := []string{
		"github.com/FangcunMount/qs-server/internal/apiserver/infra",
		"github.com/redis/",
		"github.com/go-redis/",
		"go.mongodb.org/mongo-driver",
		"gorm.io/",
	}

	err := filepath.WalkDir(domainRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			for _, forbidden := range forbiddenImports {
				if strings.HasPrefix(importPath, forbidden) {
					rel := filepath.ToSlash(mustRel(t, root, path))
					t.Fatalf("%s imports %s; domain packages must stay free of infrastructure drivers", rel, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestActorDomainDoesNotImportUpperLayers(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	actorDomainRoot := filepath.Join(root, "internal", "apiserver", "domain", "actor")
	forbiddenImports := map[string]string{
		"github.com/FangcunMount/iam/":                                      "IAM generated packages",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/": "application layer",
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/":       "infrastructure layer",
		"github.com/FangcunMount/qs-server/internal/apiserver/port/":        "application-facing ports",
		"github.com/FangcunMount/qs-server/internal/apiserver/transport/":   "transport layer",
		"github.com/go-redis/":                                              "Redis driver",
		"github.com/redis/":                                                 "Redis driver",
		"go.mongodb.org/mongo-driver":                                       "Mongo driver",
		"gorm.io/":                                                          "GORM driver",
	}

	err := filepath.WalkDir(actorDomainRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			for forbidden, description := range forbiddenImports {
				if strings.HasPrefix(importPath, forbidden) {
					rel := filepath.ToSlash(mustRel(t, root, path))
					t.Fatalf("%s imports %s; actor domain must not depend on %s", rel, importPath, description)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestActorDomainRepositoriesDoNotExposeReadModels(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	actorDomainRoot := filepath.Join(root, "internal", "apiserver", "domain", "actor")
	forbiddenMethods := map[string]string{
		"FindByIDs":                      "bulk read model lookup",
		"FindByOrgAndName":               "search read model lookup",
		"ListByOrg":                      "list read model query",
		"ListByOrgAndIDs":                "scoped list read model query",
		"ListByTags":                     "tag list read model query",
		"ListKeyFocus":                   "key-focus list read model query",
		"ListByProfileIDs":               "profile list read model query",
		"Count":                          "count read model query",
		"CountByOrgAndIDs":               "scoped count read model query",
		"ListByRole":                     "role list read model query",
		"ListActiveByClinician":          "relation list read model query",
		"ListHistoryByClinician":         "relation history read model query",
		"CountActiveByClinician":         "relation count read model query",
		"ListActiveByTestee":             "testee relation read model query",
		"ListHistoryByTestee":            "testee relation history read model query",
		"HasActiveRelationForTestee":     "access read model query",
		"ListActiveTesteeIDsByClinician": "scope read model query",
		"ListByClinician":                "assessment entry read model query",
		"CountByClinician":               "assessment entry count read model query",
	}

	fset := token.NewFileSet()
	err := filepath.WalkDir(actorDomainRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range parsed.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Name.Name != "Repository" {
					continue
				}
				iface, ok := typeSpec.Type.(*ast.InterfaceType)
				if !ok {
					continue
				}
				for _, method := range iface.Methods.List {
					if len(method.Names) == 0 {
						continue
					}
					methodName := method.Names[0].Name
					if reason, ok := forbiddenMethods[methodName]; ok {
						rel := filepath.ToSlash(mustRel(t, root, path))
						t.Fatalf("%s Repository.%s exposes %s; actor read queries must live in port/actorreadmodel", rel, methodName, reason)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func mustRel(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}
