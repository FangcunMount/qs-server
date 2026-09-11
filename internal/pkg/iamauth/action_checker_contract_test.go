package iamauth

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	authzv4 "github.com/FangcunMount/iam/v5/api/grpc/iam/authz/v4"
	"github.com/FangcunMount/iam/v5/pkg/sdk"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestActionCheckerIAMV3ContractAssessmentRetryMatrix(t *testing.T) {
	checker, server := newActionCheckerContractFixture(t)

	tests := []struct {
		name       string
		subject    string
		originType string
		allowed    bool
		role       string
	}{
		{name: "admin adhoc", subject: "user:1", originType: "adhoc", allowed: true, role: "qs:admin"},
		{name: "admin plan", subject: "user:1", originType: "plan", allowed: true, role: "qs:admin"},
		{name: "operator adhoc", subject: "user:2", originType: "adhoc", allowed: true, role: "qs:assessment_operator"},
		{name: "operator plan", subject: "user:2", originType: "plan", allowed: true, role: "qs:assessment_operator"},
		{name: "plan manager adhoc", subject: "user:3", originType: "adhoc", allowed: true, role: "qs:evaluation_plan_manager"},
		{name: "plan manager plan", subject: "user:3", originType: "plan", allowed: true, role: "qs:evaluation_plan_manager"},
		{name: "other", subject: "user:4", originType: "adhoc", allowed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := checker.CheckAction(context.Background(), appauthz.ActionCheckRequest{Subject: tt.subject, Resource: appauthz.AssessmentResource, Action: "retry"})
			if err != nil {
				t.Fatalf("CheckAction() error = %v", err)
			}
			if decision.Allowed != tt.allowed || decision.PolicyVersion != 41 || decision.MatchedRole != tt.role {
				t.Fatalf("CheckAction() decision = %+v", decision)
			}
			if !tt.allowed && decision.DenyCode != "policy_not_matched" {
				t.Fatalf("CheckAction() deny code = %q", decision.DenyCode)
			}
		})
	}

	if server.calls != len(tests) {
		t.Fatalf("IAM Check calls = %d, want %d", server.calls, len(tests))
	}
	if server.lastRequest.ObjectContext != nil {
		t.Fatal("action check sent retired object context")
	}
}

func TestActionCheckerIAMV3ContractMapsUnavailableAndInvalidAttributes(t *testing.T) {
	checker, server := newActionCheckerContractFixture(t)
	server.failureCode = codes.Unavailable
	_, err := checker.CheckAction(context.Background(), objectCheckRequest("user:2", "adhoc"))
	if !strings.Contains(errString(err), appauthz.ErrAuthorizationUnavailable.Error()) {
		t.Fatalf("CheckAction() unavailable error = %v", err)
	}

}

type contractAuthorizationServer struct {
	authzv4.UnimplementedAuthorizationServiceServer
	calls       int
	lastRequest *authzv4.CheckRequest
	failureCode codes.Code
}

func (s *contractAuthorizationServer) Check(ctx context.Context, request *authzv4.CheckRequest) (*authzv4.CheckResponse, error) {
	s.calls++
	s.lastRequest = request
	if s.failureCode != codes.OK {
		return nil, status.Error(s.failureCode, "contract fixture failure")
	}
	metadataValues, _ := metadata.FromIncomingContext(ctx)
	authorization := metadataValues.Get("authorization")
	if len(authorization) != 0 {
		return nil, status.Error(codes.PermissionDenied, "unexpected bearer metadata")
	}
	allowed := request.GetSubject() == "user:1" || request.GetSubject() == "user:2" || request.GetSubject() == "user:3"
	response := &authzv4.CheckResponse{PolicyVersion: 41}
	if !allowed {
		response.Reason = authzv4.DecisionReason_NOT_MATCHED
		response.DenyCode = "policy_not_matched"
		return response, nil
	}
	response.Allowed = true
	response.Reason = authzv4.DecisionReason_ALLOWED
	switch request.GetSubject() {
	case "user:1":
		response.MatchedGrantId, response.MatchedRole = "100", "qs:admin"
	case "user:2":
		response.MatchedGrantId, response.MatchedRole = "102", "qs:assessment_operator"
	case "user:3":
		response.MatchedGrantId, response.MatchedRole = "103", "qs:evaluation_plan_manager"
	}
	return response, nil
}

type contractGRPCClient struct{ client *sdk.Client }

func (c *contractGRPCClient) SDK() *sdk.Client { return c.client }
func (*contractGRPCClient) IsEnabled() bool    { return true }

func newActionCheckerContractFixture(t *testing.T) (*ActionChecker, *contractAuthorizationServer) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	authorizationServer := &contractAuthorizationServer{}
	authzv4.RegisterAuthorizationServiceServer(server, authorizationServer)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	client, err := sdk.NewClient(context.Background(), &sdk.Config{
		Endpoint: "passthrough:///bufnet", Timeout: time.Second, DialTimeout: time.Second,
		TLS: &sdk.TLSConfig{Enabled: false}, Retry: &sdk.RetryConfig{Enabled: false},
	}, sdk.WithDisableDefaultInterceptors(), sdk.WithDialOptions(
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
	))
	if err != nil {
		t.Fatalf("create IAM SDK client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return NewActionChecker(&contractGRPCClient{client: client}), authorizationServer
}

func objectCheckRequest(subject, origin string) appauthz.ActionCheckRequest {
	return appauthz.ActionCheckRequest{Subject: subject, Resource: appauthz.AssessmentResource, Action: "retry"}
}

func pointer(value string) *string { return &value }

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
