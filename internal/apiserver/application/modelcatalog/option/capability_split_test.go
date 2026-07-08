package option_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/option"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

func TestDomainCapabilityHasNoPresentationFields(t *testing.T) {
	t.Parallel()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "domain", "modelcatalog", "binding", "family_capability.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{"DisplayName", "APIKind", "PreviewSupported", "QRCodeSupported"}
	for _, decl := range parsed.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "ModelFamilyCapability" {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					for _, bad := range forbidden {
						if name.Name == bad {
							t.Fatalf("domain ModelFamilyCapability must not contain presentation field %q", bad)
						}
					}
				}
			}
		}
	}
}

func TestApplicationOptionProjectsDomainCapability(t *testing.T) {
	t.Parallel()

	for _, entry := range option.DefaultRegistry().RegisteredOptions() {
		if entry.IsProductChannel() {
			continue
		}
		cap, ok := binding.FamilyCapabilityByKind(entry.Kind)
		if !ok {
			continue
		}
		if entry.Operations.CreateSupported != cap.CreateSupported ||
			entry.Operations.ListSupported != cap.ListSupported ||
			entry.Operations.PublishSupported != cap.PublishSupported ||
			entry.Operations.BindQuestionnaire != cap.BindQuestionnaire ||
			entry.Operations.DefinitionUpdateSupported != cap.DefinitionUpdateSupported ||
			entry.Operations.RuntimeExecutable != cap.RuntimeExecutable ||
			entry.Operations.ExecutionPath != cap.ExecutionPath {
			t.Fatalf("kind %q operations %#v not projected from domain capability %#v", entry.Kind, entry.Operations, cap)
		}
	}
}

func TestApplicationOptionHasNoExecutionPathWriter(t *testing.T) {
	t.Parallel()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(file), "..")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range parsed.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(path, "/domain/evaluation/pipeline") {
				t.Fatalf("%s must not import evaluation pipeline for capability writes", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
