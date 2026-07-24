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
	internalpb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
	"gopkg.in/yaml.v3"
)

func TestWorkerACLContract(t *testing.T) {
	t.Parallel()

	allowed := ACLAllowedMethods()
	if len(allowed) != 7 {
		t.Fatalf("ACLAllowedMethods() count = %d, want 7", len(allowed))
	}
	assertUniqueWorkerMethods(t, allowed)

	outbound := discoverWorkerOutboundRPCMethods(t)
	if len(outbound) != 13 {
		t.Fatalf("worker outbound RPC surface count = %d, want 13", len(outbound))
	}
	assertExactWorkerMethods(t, discoverWorkerRuntimeRPCMethods(t, outbound), allowed)

	allowedSet := toWorkerMethodSet(allowed)
	denied := make([]string, 0, len(outbound)-len(allowed))
	for _, method := range outbound {
		if _, ok := allowedSet[method]; !ok {
			denied = append(denied, method)
		}
	}
	assertExactWorkerMethods(t, denied, []string{
		answersheetpb.AnswerSheetService_GetAnswerSheet_FullMethodName,
		answersheetpb.AnswerSheetService_ListAnswerSheets_FullMethodName,
		answersheetpb.AnswerSheetService_SaveAnswerSheetScores_FullMethodName,
		internalpb.InternalService_GenerateQuestionnaireQRCode_FullMethodName,
		internalpb.InternalService_GenerateScaleQRCode_FullMethodName,
		internalpb.PlanCommandService_SchedulePendingTasks_FullMethodName,
	})

	for _, configName := range []string{"grpc-acl.prod.yaml", "grpc-acl.example.yaml"} {
		configName := configName
		t.Run(configName, func(t *testing.T) {
			t.Parallel()

			acl := loadWorkerACLForContractTest(t, configName)
			permissions, ok := acl.GetServicePermissions(serviceidentity.WorkerCertificateCommonName)
			if !ok {
				t.Fatalf("ACL missing canonical certificate identity %q", serviceidentity.WorkerCertificateCommonName)
			}
			assertExactWorkerMethods(t, permissions.AllowedMethods, allowed)

			for _, method := range allowed {
				if err := acl.CheckAccess(serviceidentity.WorkerCertificateCommonName, method); err != nil {
					t.Errorf("canonical worker identity denied %s: %v", method, err)
				}
			}
			for _, method := range denied {
				if err := acl.CheckAccess(serviceidentity.WorkerCertificateCommonName, method); err == nil {
					t.Errorf("canonical worker identity unexpectedly allowed %s", method)
				}
			}

			for _, deniedIdentity := range []string{
				serviceidentity.CollectionServerCertificateCommonName,
				serviceidentity.WorkerServiceID,
				"worker.svc",
			} {
				for _, method := range allowed {
					if err := acl.CheckAccess(deniedIdentity, method); err == nil {
						t.Errorf("non-canonical identity %q unexpectedly allowed %s", deniedIdentity, method)
					}
				}
			}

			collectionOnly := answersheetpb.AnswerSheetService_SaveAnswerSheet_FullMethodName
			if err := acl.CheckAccess(serviceidentity.WorkerCertificateCommonName, collectionOnly); err == nil {
				t.Errorf("worker identity unexpectedly allowed collection-only RPC %s", collectionOnly)
			}
			if err := acl.CheckAccess(serviceidentity.CollectionServerCertificateCommonName, allowed[0]); err == nil {
				t.Errorf("collection identity unexpectedly allowed worker-only RPC %s", allowed[0])
			}
		})
	}
}

func discoverWorkerOutboundRPCMethods(t *testing.T) []string {
	t.Helper()

	type clientFile struct {
		name              string
		serviceByReceiver map[string]string
	}
	files := []clientFile{
		{name: "answersheet_client.go", serviceByReceiver: map[string]string{
			"AnswerSheetClient": "/answersheet.AnswerSheetService/",
		}},
		{name: "evaluation_clients.go", serviceByReceiver: map[string]string{
			"AssessmentIntakeClient":         "/evaluation.AssessmentIntakeService/",
			"EvaluationWorkerClient":         "/evaluation.EvaluationWorkerService/",
			"InterpretationAutomationClient": "/interpretation.InterpretationAutomationService/",
		}},
		{name: "internal_client.go", serviceByReceiver: map[string]string{
			"InternalClient": "/internalapi.InternalService/",
		}},
		{name: "plan_client.go", serviceByReceiver: map[string]string{
			"PlanClient": "/internalapi.PlanCommandService/",
		}},
	}

	methodSet := make(map[string]struct{}, 13)
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file.name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file.name, err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 || function.Body == nil {
				continue
			}
			receiverType := receiverTypeName(function.Recv.List[0].Type)
			prefix, ok := file.serviceByReceiver[receiverType]
			if !ok {
				continue
			}
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
				if !ok || clientField.Sel.Name != "client" {
					return true
				}
				receiver, ok := clientField.X.(*ast.Ident)
				if !ok || receiver.Name != "c" {
					return true
				}
				methodSet[prefix+method.Sel.Name] = struct{}{}
				return true
			})
		}
	}
	return sortedWorkerMethods(methodSet)
}

func discoverWorkerRuntimeRPCMethods(t *testing.T, outbound []string) []string {
	t.Helper()

	fullMethodByName := make(map[string]string, len(outbound))
	for _, fullMethod := range outbound {
		name := fullMethod[strings.LastIndex(fullMethod, "/")+1:]
		if previous, ok := fullMethodByName[name]; ok {
			t.Fatalf("RPC method name %q is ambiguous between %s and %s", name, previous, fullMethod)
		}
		fullMethodByName[name] = fullMethod
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	workerRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	methodSet := make(map[string]struct{}, len(outbound))
	err := filepath.WalkDir(workerRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == filepath.Join(workerRoot, "infra", "grpcclient") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			method, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if fullMethod, ok := fullMethodByName[method.Sel.Name]; ok {
				methodSet[fullMethod] = struct{}{}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("discover worker runtime RPC methods: %v", err)
	}
	return sortedWorkerMethods(methodSet)
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

func loadWorkerACLForContractTest(t *testing.T, configName string) *basegrpc.ServiceACL {
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

func sortedWorkerMethods(methodSet map[string]struct{}) []string {
	methods := make([]string, 0, len(methodSet))
	for method := range methodSet {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return methods
}

func toWorkerMethodSet(methods []string) map[string]struct{} {
	result := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		result[method] = struct{}{}
	}
	return result
}

func assertUniqueWorkerMethods(t *testing.T, methods []string) {
	t.Helper()

	seen := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		if _, ok := seen[method]; ok {
			t.Fatalf("duplicate ACL method %q", method)
		}
		seen[method] = struct{}{}
	}
}

func assertExactWorkerMethods(t *testing.T, actual, expected []string) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("actual method count = %d, want %d; actual=%v expected=%v", len(actual), len(expected), actual, expected)
	}
	actualSet := toWorkerMethodSet(actual)
	for _, method := range expected {
		if _, ok := actualSet[method]; !ok {
			t.Errorf("method set missing %s", method)
		}
	}
}
