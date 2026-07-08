package definition_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestScoringDefinitionSeamContractMatchesScaleDefinition(t *testing.T) {
	t.Parallel()

	scaleRoot := scaleDefinitionRoot(t)
	scoringRoot := filepath.Join(filepath.Dir(scaleRoot), "..", "scoring", "definition")

	scaleSyms := exportedSymbols(t, scaleRoot)
	scoringSyms := exportedSymbols(t, scoringRoot)

	assertSymbolEquivalence(t, "definition", scaleSyms, scoringSyms)
}

func TestScoringDefinitionHotRankSeamContractMatchesScaleDefinition(t *testing.T) {
	t.Parallel()

	scaleRoot := filepath.Join(scaleDefinitionRoot(t), "hotrank")
	scoringRoot := filepath.Join(filepath.Dir(scaleDefinitionRoot(t)), "..", "scoring", "definition", "hotrank")

	scaleSyms := exportedSymbols(t, scaleRoot)
	scoringSyms := exportedSymbols(t, scoringRoot)

	assertSymbolEquivalence(t, "hotrank", scaleSyms, scoringSyms)
}

func scaleDefinitionRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "scale", "definition"))
}

func assertSymbolEquivalence(t *testing.T, label string, want, got map[string]string) {
	t.Helper()

	var missing, extra []string
	for name := range want {
		if _, ok := got[name]; !ok {
			missing = append(missing, name)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("%s seam drift: missing=%v extra=%v", label, missing, extra)
	}
}

func exportedSymbols(t *testing.T, dir string) map[string]string {
	t.Helper()

	fset := token.NewFileSet()
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("Glob(%s): %v", dir, err)
	}

	out := make(map[string]string)
	for _, file := range matches {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", file, err)
		}
		for _, decl := range parsed.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							out[s.Name.Name] = "type"
						}
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if !name.IsExported() {
								continue
							}
							switch d.Tok {
							case token.CONST:
								out[name.Name] = "const"
							case token.VAR:
								out[name.Name] = "var"
							}
						}
					}
				}
			case *ast.FuncDecl:
				if d.Name.IsExported() && d.Recv == nil {
					out[d.Name.Name] = "func"
				}
			}
		}
	}
	return out
}
