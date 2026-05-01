package rest

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRESTTransportDoesNotDependOnLegacyRESTImplementation(t *testing.T) {
	t.Parallel()

	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "internal/apiserver/interface/restful") {
			t.Fatalf("transport/rest must own REST implementation and not import legacy interface/restful: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSurveyScaleRESTHandlersDoNotConstructDomainRules(t *testing.T) {
	t.Parallel()

	forbiddenImports := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation": "application DTO mapping",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation":  "application DTO mapping",
	}
	for _, path := range []string{
		filepath.Join("handler", "questionnaire.go"),
		filepath.Join("handler", "scale.go"),
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if replacement, ok := forbiddenImports[importPath]; ok {
				t.Fatalf("%s imports %s; survey/scale REST handlers should leave rule construction to %s", path, importPath, replacement)
			}
		}
	}
}
