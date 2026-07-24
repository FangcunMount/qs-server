package grpcclient

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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

	type clientFile struct {
		name     string
		prefixes map[string]string
	}
	files := []clientFile{
		{name: "answersheet_client.go", prefixes: map[string]string{"grpcClient": "/answersheet.AnswerSheetService/"}},
		{name: "questionnaire_client.go", prefixes: map[string]string{"grpcClient": "/questionnaire.QuestionnaireService/"}},
		{name: "assessment_model_catalog_client.go", prefixes: map[string]string{"grpcClient": "/assessmentmodel.AssessmentModelCatalogService/"}},
		{name: "evaluation_client.go", prefixes: map[string]string{
			"grpcClient":   "/evaluation.TesteeEvaluationService/",
			"reportClient": "/interpretation.ParticipantReportService/",
			"intakeClient": "/evaluation.AssessmentIntakeService/",
		}},
		{name: "actor_client.go", prefixes: map[string]string{"client": "/actor.ActorService/"}},
	}

	methodSet := make(map[string]struct{}, 24)
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file.name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file.name, err)
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
			receiver, ok := method.X.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			receiverName, ok := receiver.X.(*ast.Ident)
			if !ok || receiverName.Name != "c" {
				return true
			}
			prefix, ok := file.prefixes[receiver.Sel.Name]
			if ok {
				methodSet[prefix+method.Sel.Name] = struct{}{}
			}
			return true
		})
	}
	methods := make([]string, 0, len(methodSet))
	for method := range methodSet {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return methods
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
