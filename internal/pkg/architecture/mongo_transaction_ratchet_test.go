package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestProductionMongoRunnersDeclareLimiterAndStableBoundary(t *testing.T) {
	root := repoRoot(t)
	want := map[string]bool{
		"answersheet_submit":               false,
		"questionnaire_lifecycle":          false,
		"assessment_release":               false,
		"interpretation_start":             false,
		"interpretation_commit":            false,
		"interpretation_retry":             false,
		"ai_explanation_lifecycle":         false,
		"ai_explanation_prompt_evaluation": false,
	}
	fset := token.NewFileSet()
	walkGoFiles(t, filepath.Join(root, "internal/apiserver/container/modules"), func(path, text string) {
		if strings.HasSuffix(path, "_test.go") || !strings.Contains(text, "NewMongoRunner(") {
			return
		}
		file, err := parser.ParseFile(fset, path, text, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", mustRel(t, root, path), err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || mongoRunnerCallName(call.Fun) != "NewMongoRunner" {
				return true
			}
			if len(call.Args) != 2 {
				t.Fatalf("%s must construct Mongo runner with db and MongoRunnerOptions", mustRel(t, root, path))
			}
			opts, ok := call.Args[1].(*ast.CompositeLit)
			if !ok {
				t.Fatalf("%s must construct MongoRunnerOptions inline for architecture verification", mustRel(t, root, path))
			}
			boundary := ""
			hasLimiter := false
			for _, element := range opts.Elts {
				field, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, _ := field.Key.(*ast.Ident)
				if key == nil {
					continue
				}
				switch key.Name {
				case "Boundary":
					literal, _ := field.Value.(*ast.BasicLit)
					if literal != nil {
						boundary, _ = strconv.Unquote(literal.Value)
					}
				case "Limiter":
					ident, isIdent := field.Value.(*ast.Ident)
					hasLimiter = !isIdent || ident.Name != "nil"
				}
			}
			if _, ok := want[boundary]; !ok {
				t.Fatalf("%s creates Mongo runner with unsupported boundary %q", mustRel(t, root, path), boundary)
			}
			if !hasLimiter {
				t.Fatalf("%s creates Mongo runner %q without limiter", mustRel(t, root, path), boundary)
			}
			want[boundary] = true
			return true
		})
	})
	for boundary, found := range want {
		if !found {
			t.Errorf("production Mongo transaction boundary %q is not wired", boundary)
		}
	}
}

func TestTransactionalMongoRepositoriesDoNotBypassAdmissionPrimitives(t *testing.T) {
	root := repoRoot(t)
	forbidden := []string{
		".Collection().InsertOne(",
		".Collection().UpdateOne(",
		".Collection().UpdateMany(",
		".Collection().DeleteOne(",
		".Collection().DeleteMany(",
		".Collection().ReplaceOne(",
		".Collection().FindOneAndUpdate(",
		").InsertOne(",
		").InsertMany(",
		").UpdateOne(",
		").UpdateMany(",
		").DeleteOne(",
		").DeleteMany(",
		").ReplaceOne(",
		").FindOneAndUpdate(",
		").BulkWrite(",
	}
	for _, relRoot := range []string{
		"internal/apiserver/infra/mongo/answersheet",
		"internal/apiserver/infra/mongo/questionnaire",
		"internal/apiserver/infra/mongo/modelcatalog",
		"internal/apiserver/infra/mongo/interpretation",
	} {
		walkGoFiles(t, filepath.Join(root, relRoot), func(path, text string) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}
			for _, token := range forbidden {
				if strings.Contains(text, token) {
					t.Fatalf("%s bypasses BaseRepository admission with %q", mustRel(t, root, path), token)
				}
			}
		})
	}
}

func TestMongoOutboxTransactionalStageUsesSessionAwareAdmissionPrimitive(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "internal/apiserver/infra/mongo/eventoutbox/store.go")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, raw := range []string{"s.coll.InsertMany(txCtx", "s.coll.InsertMany(ctx"} {
		if strings.Contains(text, raw) {
			t.Fatalf("%s bypasses session-aware transaction write primitive with %q", mustRel(t, root, path), raw)
		}
	}
	if !strings.Contains(text, "s.transactionWrites.InsertMany(") {
		t.Fatalf("%s does not route transactional staging through BaseRepository", mustRel(t, root, path))
	}
}

func TestMongoConsistencyScannerCannotMutateBusinessCollections(t *testing.T) {
	root := repoRoot(t)
	forbidden := []string{".InsertOne(", ".InsertMany(", ".UpdateOne(", ".UpdateMany(", ".DeleteOne(", ".DeleteMany(", ".ReplaceOne(", ".BulkWrite(", ".FindOneAndUpdate("}
	walkGoFiles(t, filepath.Join(root, "internal/apiserver/infra/mongo/mongoconsistency"), func(path, text string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Fatalf("%s gives read-only audit a Mongo mutation primitive %q", mustRel(t, root, path), token)
			}
		}
	})
}

func mongoRunnerCallName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}
