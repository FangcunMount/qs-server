package iamauth

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	authzv3 "github.com/FangcunMount/iam/v3/api/grpc/iam/authz/v3"
	"github.com/FangcunMount/iam/v3/pkg/sdk"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestObjectCheckerIAMV3ContractAssessmentRetryMatrix(t *testing.T) {
	checker, server := newObjectCheckerContractFixture(t)

	tests := []struct {
		name       string
		subject    string
		originType string
		allowed    bool
		role       string
	}{
		{name: "admin adhoc", subject: "user:1", originType: "adhoc", allowed: true, role: "qs:admin"},
		{name: "admin plan", subject: "user:1", originType: "plan", allowed: true, role: "qs:admin"},
		{name: "evaluator adhoc", subject: "user:2", originType: "adhoc", allowed: true, role: "qs:evaluator"},
		{name: "evaluator plan", subject: "user:2", originType: "plan", allowed: false},
		{name: "plan manager adhoc", subject: "user:3", originType: "adhoc", allowed: false},
		{name: "plan manager plan", subject: "user:3", originType: "plan", allowed: true, role: "qs:evaluation_plan_manager"},
		{name: "other", subject: "user:4", originType: "adhoc", allowed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := checker.CheckObject(context.Background(), appauthz.ObjectCheckRequest{
				Subject: tt.subject, Domain: "fangcun", Resource: appauthz.AssessmentResource,
				Action: "retry", ObjectID: "assessment-1",
				Attributes: map[string]appauthz.ObjectAttribute{
					appauthz.ObjectOriginTypeAttribute: appauthz.StringAttribute(tt.originType),
				},
			})
			if err != nil {
				t.Fatalf("CheckObject() error = %v", err)
			}
			if decision.Allowed != tt.allowed || decision.PolicyVersion != 41 || decision.MatchedRole != tt.role {
				t.Fatalf("CheckObject() decision = %+v", decision)
			}
			if !tt.allowed && decision.DenyCode != "policy_not_matched" {
				t.Fatalf("CheckObject() deny code = %q", decision.DenyCode)
			}
		})
	}

	if server.calls != len(tests) {
		t.Fatalf("IAM Check calls = %d, want %d", server.calls, len(tests))
	}
	if server.lastRequest.GetObjectContext().GetObjectId() != "assessment-1" {
		t.Fatalf("IAM object id = %q", server.lastRequest.GetObjectContext().GetObjectId())
	}
	attributes := server.lastRequest.GetObjectContext().GetAttributes()
	if len(attributes) != 1 || attributes[0].GetKey() != "object.origin_type" || attributes[0].GetStringValue() != "adhoc" {
		t.Fatalf("IAM typed attributes = %#v", attributes)
	}
}

func TestObjectCheckerIAMV3ContractMapsUnavailableAndInvalidAttributes(t *testing.T) {
	checker, server := newObjectCheckerContractFixture(t)
	server.failureCode = codes.Unavailable
	_, err := checker.CheckObject(context.Background(), objectCheckRequest("user:2", "adhoc"))
	if !strings.Contains(errString(err), appauthz.ErrAuthorizationUnavailable.Error()) {
		t.Fatalf("CheckObject() unavailable error = %v", err)
	}

	server.failureCode = codes.OK
	invalid := objectCheckRequest("user:2", "adhoc")
	value := int64(1)
	invalid.Attributes[appauthz.ObjectOriginTypeAttribute] = appauthz.ObjectAttribute{
		String: pointer("adhoc"), Int64: &value,
	}
	_, err = checker.CheckObject(context.Background(), invalid)
	if !strings.Contains(errString(err), appauthz.ErrAuthorizationContract.Error()) || server.calls != 1 {
		t.Fatalf("CheckObject() contract error = %v, IAM calls = %d", err, server.calls)
	}
}

type contractAuthorizationServer struct {
	authzv3.UnimplementedAuthorizationServiceServer
	calls       int
	lastRequest *authzv3.CheckRequest
	failureCode codes.Code
}

func (s *contractAuthorizationServer) Check(ctx context.Context, request *authzv3.CheckRequest) (*authzv3.CheckResponse, error) {
	s.calls++
	s.lastRequest = request
	if s.failureCode != codes.OK {
		return nil, status.Error(s.failureCode, "contract fixture failure")
	}
	metadataValues, ok := metadata.FromIncomingContext(ctx)
	authorization := metadataValues.Get("authorization")
	if !ok || len(authorization) != 1 || authorization[0] != "Bearer contract-service-token" {
		return nil, status.Error(codes.PermissionDenied, "service credential is required")
	}
	originType := ""
	if attributes := request.GetObjectContext().GetAttributes(); len(attributes) == 1 {
		originType = attributes[0].GetStringValue()
	}
	allowed := request.GetSubject() == "user:1" ||
		request.GetSubject() == "user:2" && originType == "adhoc" ||
		request.GetSubject() == "user:3" && originType == "plan"
	response := &authzv3.CheckResponse{PolicyVersion: 41}
	if !allowed {
		response.Reason = authzv3.DecisionReason_NOT_MATCHED
		response.DenyCode = "policy_not_matched"
		return response, nil
	}
	response.Allowed = true
	response.Reason = authzv3.DecisionReason_ALLOWED
	switch request.GetSubject() {
	case "user:1":
		response.MatchedGrantId, response.MatchedRole = "100", "qs:admin"
	case "user:2":
		response.MatchedGrantId, response.MatchedRole = "102", "qs:evaluator"
	case "user:3":
		response.MatchedGrantId, response.MatchedRole = "103", "qs:evaluation_plan_manager"
	}
	return response, nil
}

type contractGRPCClient struct{ client *sdk.Client }

func (c *contractGRPCClient) SDK() *sdk.Client { return c.client }
func (*contractGRPCClient) IsEnabled() bool    { return true }

type contractTokenProvider struct{}

func (contractTokenProvider) GetToken(context.Context) (string, error) {
	return "contract-service-token", nil
}

func newObjectCheckerContractFixture(t *testing.T) (*ObjectChecker, *contractAuthorizationServer) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	authorizationServer := &contractAuthorizationServer{}
	authzv3.RegisterAuthorizationServiceServer(server, authorizationServer)
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
	return NewObjectChecker(&contractGRPCClient{client: client}, contractTokenProvider{}), authorizationServer
}

func objectCheckRequest(subject, originType string) appauthz.ObjectCheckRequest {
	return appauthz.ObjectCheckRequest{
		Subject: subject, Domain: "fangcun", Resource: appauthz.AssessmentResource,
		Action: "retry", ObjectID: "assessment-1",
		Attributes: map[string]appauthz.ObjectAttribute{
			appauthz.ObjectOriginTypeAttribute: appauthz.StringAttribute(originType),
		},
	}
}

func pointer(value string) *string { return &value }

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
