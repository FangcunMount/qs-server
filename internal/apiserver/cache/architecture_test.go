package cache_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBusinessCachePackagesStayModuleOwned(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(file)
	owners := []string{"survey", "modelcatalog", "evaluation", "actor", "plan", "statistics"}
	for _, owner := range owners {
		files, err := filepath.Glob(filepath.Join(root, owner, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imported := range parsed.Imports {
				path := strings.Trim(imported.Path.Value, `"`)
				marker := "/internal/apiserver/domain/"
				if idx := strings.Index(path, marker); idx >= 0 {
					rest := strings.TrimPrefix(path[idx+len(marker):], "/")
					if !strings.HasPrefix(rest, owner+"/") && rest != owner {
						t.Fatalf("%s imports foreign domain %s", name, path)
					}
				}
			}
		}
	}
}

func TestFlatAdapterPackageDoesNotReturn(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "adapter")
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("flat adapter package returned: %s", path)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestAdapterKitDoesNotImportBusinessPackages(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	files, err := filepath.Glob(filepath.Join(filepath.Dir(file), "internal", "adapterkit", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, imported := range parsed.Imports {
			path := strings.Trim(imported.Path.Value, `"`)
			for _, marker := range []string{"/internal/apiserver/domain/", "/internal/apiserver/application/", "/internal/apiserver/port/"} {
				if strings.Contains(path, marker) {
					t.Fatalf("%s imports business package %s", name, path)
				}
			}
		}
	}
}

func TestDomainAndApplicationDoNotImportConcreteCacheAdapters(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	apiserverRoot := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	for _, area := range []string{"domain", "application"} {
		err := filepath.Walk(filepath.Join(apiserverRoot, area), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if parseErr != nil {
				return parseErr
			}
			for _, imported := range parsed.Imports {
				importPath := strings.Trim(imported.Path.Value, `"`)
				for _, owner := range []string{"survey", "modelcatalog", "evaluation", "actor", "plan", "statistics"} {
					if strings.HasPrefix(importPath, "github.com/FangcunMount/qs-server/internal/apiserver/cache/"+owner) {
						t.Fatalf("%s imports concrete cache adapter %s", path, importPath)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestPublishedModelCacheHasOneProductionCompositionRoot(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	apiserverRoot := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	var callSites []string
	err := filepath.Walk(filepath.Join(apiserverRoot, "container"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), "NewCachedPublishedModelStore(") {
			callSites = append(callSites, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(callSites) != 1 || !strings.HasSuffix(callSites[0], "/container/modules/modelcatalog/wire.go") {
		t.Fatalf("published-model cache production constructors = %v, want modelcatalog/wire.go only", callSites)
	}
}
