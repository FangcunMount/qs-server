package definition

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestDomainScaleRootDoesNotImportApplicationInfraOrEvaluationPipeline(t *testing.T) {
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("Glob returned error: %v", err)
	}
	for _, file := range matches {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("ParseFile(%s) returned error: %v", file, err)
		}
		for _, imp := range parsed.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(path, "/internal/apiserver/application/") ||
				strings.Contains(path, "/internal/apiserver/infra/") ||
				strings.Contains(path, "/internal/apiserver/port/evaluationinput") ||
				strings.Contains(path, "/application/evaluation/execute") {
				t.Fatalf("domain/assessmentmodel/scale/definition must not import %s in %s", path, file)
			}
		}
	}
}
