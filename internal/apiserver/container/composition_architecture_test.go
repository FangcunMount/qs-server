package container

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

func TestAPIServerCompositionSettersAreAllowlisted(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	allowedDefinitions := map[string]string{
		"internal/apiserver/container/assembler/evaluation.go:EvaluationModule.SetScaleRepository": "compat_legacy",
		"internal/apiserver/container/assembler/evaluation.go:EvaluationModule.SetQRCodeService":   "compat_noop",
		"internal/apiserver/container/assembler/scale.go:ScaleModule.SetQRCodeService":             "qrcode_fanout",
		"internal/apiserver/container/assembler/survey.go:SurveyModule.SetScaleRepository":         "post_wire_dependency",
		"internal/apiserver/container/assembler/survey.go:SurveyModule.SetQRCodeService":           "qrcode_fanout",
	}

	got := map[string]struct{}{}
	scanGoFiles(t, filepath.Join(root, "internal", "apiserver", "container", "assembler"), func(path string, file *ast.File) {
		rel := filepath.ToSlash(mustRel(t, root, path))
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !strings.HasPrefix(fn.Name.Name, "Set") {
				continue
			}
			recv := receiverTypeName(fn)
			key := rel + ":" + recv + "." + fn.Name.Name
			if _, ok := allowedDefinitions[key]; !ok {
				t.Fatalf("%s is a new composition setter; add it to ModuleGraph/PostWire with a tested reason before allowing it", key)
			}
			got[key] = struct{}{}
		}
	})
	for key, reason := range allowedDefinitions {
		if strings.TrimSpace(reason) == "" {
			t.Fatalf("%s has an empty allowlist reason", key)
		}
		if _, ok := got[key]; !ok {
			t.Fatalf("allowlisted composition setter %s no longer exists; remove it from the allowlist", key)
		}
	}
}

func TestAPIServerPostWireCallsStayInModuleGraph(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	containerRoot := filepath.Join(root, "internal", "apiserver", "container")
	allowedFiles := map[string]struct{}{
		"internal/apiserver/container/module_graph.go": {},
	}

	scanGoSourceFiles(t, containerRoot, func(path, content string) {
		rel := filepath.ToSlash(mustRel(t, root, path))
		if strings.HasPrefix(rel, "internal/apiserver/container/assembler/") {
			return
		}
		if _, ok := allowedFiles[rel]; ok {
			return
		}
		for _, token := range []string{
			".SetEvaluationServices(",
			".SetTesteeAccessService(",
			".SetScaleRepository(",
			".SetQRCodeService(",
			".SetWarmupCoordinator(",
		} {
			if strings.Contains(content, token) {
				t.Fatalf("%s calls %s; cross-module wiring must live in module_graph.go", rel, token)
			}
		}
	})
}

func TestActorAssemblerDoesNotImportRESTHandlers(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "container", "assembler", "actor.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imported := range parsed.Imports {
		importPath := strings.Trim(imported.Path.Value, `"`)
		if strings.HasPrefix(importPath, "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler") {
			t.Fatalf("internal/apiserver/container/assembler/actor.go imports %s; actor REST handlers must be composed inside transport/rest", importPath)
		}
	}
}

func TestActorModuleDoesNotExposeRepositories(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "container", "assembler", "actor.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range parsed.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "ActorModule" {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					if strings.HasSuffix(name.Name, "Repo") {
						t.Fatalf("ActorModule exposes %s; actor repositories must stay private to the actor assembler", name.Name)
					}
				}
			}
		}
	}
}

func TestActorTransportsDoNotDependOnActorRepositoryImplementations(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, relRoot := range []string{
		filepath.Join("internal", "apiserver", "transport", "rest"),
		filepath.Join("internal", "apiserver", "transport", "grpc"),
	} {
		scanGoFiles(t, filepath.Join(root, relRoot), func(path string, file *ast.File) {
			for _, imported := range file.Imports {
				importPath := strings.Trim(imported.Path.Value, `"`)
				switch {
				case strings.HasPrefix(importPath, "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"):
					rel := filepath.ToSlash(mustRel(t, root, path))
					t.Fatalf("%s imports %s; actor transport must consume application ports/read models, not actor infra repositories", rel, importPath)
				case strings.HasPrefix(importPath, "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"):
					rel := filepath.ToSlash(mustRel(t, root, path))
					t.Fatalf("%s imports %s; actor transport must not depend on cached actor repositories", rel, importPath)
				}
			}
		})
	}
}

func TestActorTransportsDoNotReferToActorDomainRepositories(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, relRoot := range []string{
		filepath.Join("internal", "apiserver", "transport", "rest"),
		filepath.Join("internal", "apiserver", "transport", "grpc"),
	} {
		scanGoSourceFiles(t, filepath.Join(root, relRoot), func(path, content string) {
			parsed, err := parser.ParseFile(token.NewFileSet(), path, []byte(content), parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imported := range parsed.Imports {
				importPath := strings.Trim(imported.Path.Value, `"`)
				if !strings.HasPrefix(importPath, "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/") {
					continue
				}
				alias := importAlias(imported, importPath)
				if alias == "" {
					continue
				}
				if strings.Contains(content, alias+".Repository") {
					rel := filepath.ToSlash(mustRel(t, root, path))
					t.Fatalf("%s refers to %s.Repository; actor transport must not hold actor domain repositories", rel, alias)
				}
			}
		})
	}
}

func receiverTypeName(fn *ast.FuncDecl) string {
	if fn == nil || fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	switch expr := fn.Recv.List[0].Type.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		if ident, ok := expr.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

func importAlias(imported *ast.ImportSpec, importPath string) string {
	if imported.Name != nil {
		if imported.Name.Name == "_" || imported.Name.Name == "." {
			return ""
		}
		return imported.Name.Name
	}
	if idx := strings.LastIndex(importPath, "/"); idx >= 0 {
		return importPath[idx+1:]
	}
	return importPath
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

func scanGoFiles(t *testing.T, root string, visit func(path string, file *ast.File)) {
	t.Helper()
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		visit(path, file)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func scanGoSourceFiles(t *testing.T, root string, visit func(path, content string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		bytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		visit(path, string(bytes))
		return nil
	})
	if err != nil {
		t.Fatal(err)
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
