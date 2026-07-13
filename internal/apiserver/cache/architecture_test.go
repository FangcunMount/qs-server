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
