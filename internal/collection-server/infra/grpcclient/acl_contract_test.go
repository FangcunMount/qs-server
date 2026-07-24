package grpcclient

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	basegrpc "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	answersheetpb "github.com/FangcunMount/qs-server/api/grpc/gen/answersheet"
	evaluationpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
	"gopkg.in/yaml.v3"
)

func TestCollectionServerACLContract(t *testing.T) {
	t.Parallel()

	allowed := ACLAllowedMethods()
	if len(allowed) != 24 {
		t.Fatalf("ACLAllowedMethods() count = %d, want 24", len(allowed))
	}
	assertUniqueMethods(t, allowed)
	assertExactMethods(t, allowed, discoverOutboundRPCMethods(t))

	for _, configName := range []string{"grpc-acl.prod.yaml", "grpc-acl.example.yaml"} {
		configName := configName
		t.Run(configName, func(t *testing.T) {
			t.Parallel()

			acl := loadACLForContractTest(t, configName)
			permissions, ok := acl.GetServicePermissions(serviceidentity.CollectionServerCertificateCommonName)
			if !ok {
				t.Fatalf("ACL missing canonical certificate identity %q", serviceidentity.CollectionServerCertificateCommonName)
			}
			assertExactMethods(t, permissions.AllowedMethods, allowed)

			for _, method := range allowed {
				if err := acl.CheckAccess(serviceidentity.CollectionServerCertificateCommonName, method); err != nil {
					t.Errorf("canonical collection identity denied %s: %v", method, err)
				}
			}

			for _, denied := range []string{
				answersheetpb.AnswerSheetService_SaveAnswerSheetScores_FullMethodName,
				evaluationpb.AssessmentIntakeService_EnsureAssessment_FullMethodName,
				interpretationpb.ParticipantReportService_ListMyReports_FullMethodName,
				interpretationpb.InterpretationAutomationService_GenerateReportFromOutcome_FullMethodName,
			} {
				if err := acl.CheckAccess(serviceidentity.CollectionServerCertificateCommonName, denied); err == nil {
					t.Errorf("canonical collection identity unexpectedly allowed %s", denied)
				}
			}

			for _, deniedIdentity := range []string{"qs-collection.svc", "qs-collection", "qs-worker.svc"} {
				if err := acl.CheckAccess(deniedIdentity, allowed[0]); err == nil {
					t.Errorf("non-canonical identity %q unexpectedly allowed", deniedIdentity)
				}
			}
		})
	}
}

func discoverOutboundRPCMethods(t *testing.T) []string {
	t.Helper()

	servicePrefixByClientType := map[string]string{
		"ActorServiceClient":                  "/actor.ActorService/",
		"AnswerSheetServiceClient":            "/answersheet.AnswerSheetService/",
		"AssessmentIntakeServiceClient":       "/evaluation.AssessmentIntakeService/",
		"AssessmentModelCatalogServiceClient": "/assessmentmodel.AssessmentModelCatalogService/",
		"ParticipantReportServiceClient":      "/interpretation.ParticipantReportService/",
		"QuestionnaireServiceClient":          "/questionnaire.QuestionnaireService/",
		"TesteeEvaluationServiceClient":       "/evaluation.TesteeEvaluationService/",
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	packageDir := filepath.Dir(currentFile)
	parsedFiles := parseNonTestGoFiles(t, packageDir)
	serviceByStructField := discoverGeneratedClientFields(t, parsedFiles, servicePrefixByClientType)

	methodSet := make(map[string]struct{}, 24)
	for _, parsed := range parsedFiles {
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 || function.Body == nil {
				continue
			}
			fields := serviceByStructField[receiverTypeName(function.Recv.List[0].Type)]
			if len(fields) == 0 || len(function.Recv.List[0].Names) != 1 {
				continue
			}
			receiverName := function.Recv.List[0].Names[0].Name
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				method, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				clientField, ok := method.X.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				receiver, ok := clientField.X.(*ast.Ident)
				if !ok || receiver.Name != receiverName {
					return true
				}
				if prefix, ok := fields[clientField.Sel.Name]; ok {
					methodSet[prefix+method.Sel.Name] = struct{}{}
				}
				return true
			})
		}
	}
	methods := make([]string, 0, len(methodSet))
	for method := range methodSet {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return methods
}

func parseNonTestGoFiles(t *testing.T, packageDir string) []*ast.File {
	t.Helper()

	entries, err := os.ReadDir(packageDir)
	if err != nil {
		t.Fatalf("read grpcclient package: %v", err)
	}
	parsedFiles := make([]*ast.File, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(packageDir, entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		parsedFiles = append(parsedFiles, parsed)
	}
	return parsedFiles
}

func discoverGeneratedClientFields(
	t *testing.T,
	parsedFiles []*ast.File,
	servicePrefixByClientType map[string]string,
) map[string]map[string]string {
	t.Helper()

	serviceByStructField := make(map[string]map[string]string)
	for _, parsed := range parsedFiles {
		for _, declaration := range parsed.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range structType.Fields.List {
					clientType := selectedTypeName(field.Type)
					if !strings.HasSuffix(clientType, "ServiceClient") {
						continue
					}
					prefix, known := servicePrefixByClientType[clientType]
					if !known {
						t.Fatalf("unknown generated gRPC client type %q on %s; review and map it explicitly", clientType, typeSpec.Name.Name)
					}
					if serviceByStructField[typeSpec.Name.Name] == nil {
						serviceByStructField[typeSpec.Name.Name] = make(map[string]string)
					}
					for _, name := range field.Names {
						serviceByStructField[typeSpec.Name.Name][name.Name] = prefix
					}
				}
			}
		}
	}
	return serviceByStructField
}

func selectedTypeName(expr ast.Expr) string {
	if pointer, ok := expr.(*ast.StarExpr); ok {
		expr = pointer.X
	}
	if selector, ok := expr.(*ast.SelectorExpr); ok {
		return selector.Sel.Name
	}
	return ""
}

func receiverTypeName(expr ast.Expr) string {
	if pointer, ok := expr.(*ast.StarExpr); ok {
		expr = pointer.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

func loadACLForContractTest(t *testing.T, configName string) *basegrpc.ServiceACL {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", ".."))
	data, err := os.ReadFile(filepath.Join(repoRoot, "configs", configName))
	if err != nil {
		t.Fatalf("read %s: %v", configName, err)
	}
	var cfg basegrpc.ACLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse %s: %v", configName, err)
	}
	if cfg.DefaultPolicy != "deny" {
		t.Fatalf("%s default_policy = %q, want deny", configName, cfg.DefaultPolicy)
	}
	return basegrpc.NewServiceACL(&cfg)
}

func assertUniqueMethods(t *testing.T, methods []string) {
	t.Helper()

	seen := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		if _, ok := seen[method]; ok {
			t.Fatalf("duplicate ACL method %q", method)
		}
		seen[method] = struct{}{}
	}
}

func assertExactMethods(t *testing.T, actual, expected []string) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("configured allowed method count = %d, want %d", len(actual), len(expected))
	}
	actualSet := make(map[string]struct{}, len(actual))
	for _, method := range actual {
		actualSet[method] = struct{}{}
	}
	for _, method := range expected {
		if _, ok := actualSet[method]; !ok {
			t.Errorf("configured ACL missing %s", method)
		}
	}
}
