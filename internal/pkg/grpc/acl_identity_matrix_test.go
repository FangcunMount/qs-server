package grpc_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	basegrpc "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	actorpb "github.com/FangcunMount/qs-server/api/grpc/gen/actor"
	answersheetpb "github.com/FangcunMount/qs-server/api/grpc/gen/answersheet"
	assessmentmodelpb "github.com/FangcunMount/qs-server/api/grpc/gen/assessmentmodel"
	evaluationpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	internalpb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	questionnairepb "github.com/FangcunMount/qs-server/api/grpc/gen/questionnaire"
	collectiongrpcclient "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
	workergrpcclient "github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

func TestRegisteredUnaryRPCIdentityACLMatrix(t *testing.T) {
	t.Parallel()

	methods := registeredUnaryRPCMethods(t)
	registered := toMethodSet(t, methods)
	for identity, allowed := range map[string][]string{
		serviceidentity.CollectionServerCertificateCommonName: collectiongrpcclient.ACLAllowedMethods(),
		serviceidentity.WorkerCertificateCommonName:           workergrpcclient.ACLAllowedMethods(),
	} {
		for _, method := range allowed {
			if _, ok := registered[method]; !ok {
				t.Fatalf("ACL for %q references RPC not present in the registered service descriptors: %s", identity, method)
			}
		}
	}
	acl := loadProductionACL(t)
	tests := []struct {
		name       string
		commonName string
		allowed    []string
	}{
		{
			name:       "canonical collection",
			commonName: serviceidentity.CollectionServerCertificateCommonName,
			allowed:    collectiongrpcclient.ACLAllowedMethods(),
		},
		{
			name:       "canonical worker",
			commonName: serviceidentity.WorkerCertificateCommonName,
			allowed:    workergrpcclient.ACLAllowedMethods(),
		},
		{name: "legacy collection", commonName: "qs-collection.svc"},
		{name: "bare worker", commonName: serviceidentity.WorkerServiceID},
		{name: "legacy worker", commonName: "worker.svc"},
		{name: "unknown identity", commonName: "unknown.svc"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cert := parsedClientCertificate(t, tt.commonName)
			allowed := toMethodSet(t, tt.allowed)
			for _, method := range methods {
				method := method
				t.Run(method, func(t *testing.T) {
					handlerCalled, err := invokeIdentityACLChain(tlsPeerContext(cert), acl, method)
					_, shouldAllow := allowed[method]
					if shouldAllow {
						if err != nil || !handlerCalled {
							t.Fatalf("%s denied for %q: handler_called=%t error=%v", method, tt.commonName, handlerCalled, err)
						}
						return
					}
					if status.Code(err) != codes.PermissionDenied {
						t.Fatalf("%s for %q returned %v, want PermissionDenied", method, tt.commonName, err)
					}
					if handlerCalled {
						t.Fatalf("%s for %q reached handler despite denial", method, tt.commonName)
					}
				})
			}
		})
	}
}

func TestIdentityACLDefaultSkipMethodsBypassBothInterceptors(t *testing.T) {
	t.Parallel()

	acl := loadProductionACL(t)
	for _, method := range basegrpc.DefaultSkipMethods() {
		method := method
		t.Run(method, func(t *testing.T) {
			handlerCalled, err := invokeIdentityACLChain(context.Background(), acl, method)
			if err != nil || !handlerCalled {
				t.Fatalf("default skip method %s did not bypass identity and ACL: handler_called=%t error=%v", method, handlerCalled, err)
			}
		})
	}
}

func registeredUnaryRPCMethods(t *testing.T) []string {
	t.Helper()

	descriptors := []*gogrpc.ServiceDesc{
		&actorpb.ActorService_ServiceDesc,
		&answersheetpb.AnswerSheetService_ServiceDesc,
		&assessmentmodelpb.AssessmentModelCatalogService_ServiceDesc,
		&evaluationpb.TesteeEvaluationService_ServiceDesc,
		&evaluationpb.AssessmentIntakeService_ServiceDesc,
		&evaluationpb.EvaluationWorkerService_ServiceDesc,
		&interpretationpb.ParticipantReportService_ServiceDesc,
		&interpretationpb.InterpretationAutomationService_ServiceDesc,
		&internalpb.InternalService_ServiceDesc,
		&internalpb.PlanCommandService_ServiceDesc,
		&questionnairepb.QuestionnaireService_ServiceDesc,
	}
	methodSet := make(map[string]struct{})
	for _, descriptor := range descriptors {
		if len(descriptor.Streams) != 0 {
			t.Fatalf("business service %s introduced %d streaming RPCs; define stream mTLS/ACL design before accepting it", descriptor.ServiceName, len(descriptor.Streams))
		}
		for _, method := range descriptor.Methods {
			fullMethod := "/" + descriptor.ServiceName + "/" + method.MethodName
			if _, duplicate := methodSet[fullMethod]; duplicate {
				t.Fatalf("duplicate registered RPC %s", fullMethod)
			}
			methodSet[fullMethod] = struct{}{}
		}
	}
	methods := make([]string, 0, len(methodSet))
	for method := range methodSet {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return methods
}

func loadProductionACL(t *testing.T) *basegrpc.ServiceACL {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	configPath := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "configs", "grpc-acl.prod.yaml"))
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read production ACL: %v", err)
	}
	var config basegrpc.ACLConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse production ACL: %v", err)
	}
	return basegrpc.NewServiceACL(&config)
}

func parsedClientCertificate(t *testing.T, commonName string) *x509.Certificate {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate certificate key: %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate for %q: %v", commonName, err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate for %q: %v", commonName, err)
	}
	return cert
}

func tlsPeerContext(cert *x509.Certificate) context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}},
		},
	})
}

func invokeIdentityACLChain(
	ctx context.Context,
	acl *basegrpc.ServiceACL,
	method string,
) (bool, error) {
	info := &gogrpc.UnaryServerInfo{FullMethod: method}
	mtlsInterceptor := basegrpc.MTLSInterceptor()
	aclInterceptor := basegrpc.ACLInterceptor(acl)
	handlerCalled := false
	_, err := mtlsInterceptor(ctx, nil, info, func(identityContext context.Context, request interface{}) (interface{}, error) {
		return aclInterceptor(identityContext, request, info, func(context.Context, interface{}) (interface{}, error) {
			handlerCalled = true
			return struct{}{}, nil
		})
	})
	return handlerCalled, err
}

func toMethodSet(t *testing.T, methods []string) map[string]struct{} {
	t.Helper()

	result := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		if _, duplicate := result[method]; duplicate {
			t.Fatalf("duplicate expected ACL method %s", method)
		}
		result[method] = struct{}{}
	}
	return result
}
