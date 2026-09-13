package grpcclient

import (
	"context"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type workflowRPCProbe struct {
	t              *testing.T
	verifier       *delegatedsubject.Verifier
	calls          int
	invalidContent bool
}

func (p *workflowRPCProbe) check(ctx context.Context, testeeID, assessmentID uint64, purpose string) {
	p.t.Helper()
	p.calls++
	if testeeID != 7 || assessmentID != 42 {
		p.t.Fatalf("wrong subject/report scope: %d/%d", testeeID, assessmentID)
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	tokens := md.Get(delegatedsubject.MetadataKey)
	if len(tokens) != 1 {
		p.t.Fatalf("delegated token count: %d", len(tokens))
	}
	token, err := p.verifier.Verify(tokens[0], purpose, 7)
	if err != nil || token.UserID != "12" || token.OrgID != 9 {
		p.t.Fatalf("delegation not preserved: %#v, %v", token, err)
	}
	if _, ok := ctx.Deadline(); !ok {
		p.t.Fatal("RPC has no deadline")
	}
}
func (p *workflowRPCProbe) RequestAIWorkflow(ctx context.Context, req *pb.RequestAIWorkflowRequest, _ ...grpc.CallOption) (*pb.AIWorkflowAccepted, error) {
	p.check(ctx, req.TesteeId, req.AssessmentId, delegatedsubject.PurposeAIExplanationRequest)
	if req.ReportId != 18 || req.RequestId != "a488863c-85df-45f5-aa3e-19574e27fda1" {
		p.t.Fatalf("idempotency/source identifiers changed: %v", req)
	}
	return &pb.AIWorkflowAccepted{RequestId: req.RequestId, Status: "accepted"}, nil
}
func (p *workflowRPCProbe) GetAIWorkflowSource(ctx context.Context, req *pb.GetAIWorkflowSourceRequest, _ ...grpc.CallOption) (*pb.AIWorkflowSource, error) {
	p.check(ctx, req.TesteeId, req.AssessmentId, delegatedsubject.PurposeAIExplanationCapability)
	return &pb.AIWorkflowSource{Status: "ready", ReportId: "18", SourceVersion: "sha256:source"}, nil
}
func (p *workflowRPCProbe) GetAIWorkflow(ctx context.Context, req *pb.GetAIWorkflowRequest, _ ...grpc.CallOption) (*pb.AIWorkflowResult, error) {
	p.check(ctx, req.TesteeId, req.AssessmentId, delegatedsubject.PurposeAIExplanationGet)
	content := `{"summary":"ready"}`
	if p.invalidContent {
		content = "invalid json"
	}
	return &pb.AIWorkflowResult{RequestId: req.RequestId, Status: "completed", Version: 3, ContentJson: content, ReportId: "18", SourceVersion: "sha256:source"}, nil
}
func TestWorkflowRPCPreservesDelegationAndRequestIdentity(t *testing.T) {
	opts := &delegatedsubject.Options{Enabled: true, CurrentKey: "workflow-client-test-key", TTL: time.Minute}
	signer, err := delegatedsubject.NewSignerFromOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := delegatedsubject.NewVerifierFromOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	probe := &workflowRPCProbe{t: t, verifier: verifier}
	client := &ParticipantAIExplanationClient{client: &Client{config: &ClientConfig{Timeout: time.Second}}, service: probe, signer: signer}
	ctx := context.WithValue(t.Context(), pkgmiddleware.UserClaimsContextKey{}, &pkgmiddleware.UserClaims{UserID: "12", OrgID: "9"})
	const requestID = "a488863c-85df-45f5-aa3e-19574e27fda1"
	accepted, err := client.RequestWorkflow(ctx, 7, 42, 18, requestID)
	if err != nil || accepted.RequestID != requestID {
		t.Fatalf("request: %#v %v", accepted, err)
	}
	source, err := client.GetWorkflowSource(ctx, 7, 42)
	if err != nil || source.ReportID != "18" || source.SourceVersion != "sha256:source" {
		t.Fatalf("source: %#v %v", source, err)
	}
	result, err := client.GetWorkflow(ctx, 7, 42, requestID)
	if err != nil || result.RequestID != requestID || result.Version != 3 || result.SourceVersion != source.SourceVersion {
		t.Fatalf("result: %#v %v", result, err)
	}
	probe.invalidContent = true
	if _, err = client.GetWorkflow(ctx, 7, 42, requestID); err == nil {
		t.Fatal("invalid result JSON accepted")
	}
	before := probe.calls
	if _, err = client.RequestWorkflow(t.Context(), 7, 42, 18, requestID); err == nil || probe.calls != before {
		t.Fatal("missing user identity reached RPC")
	}
}
