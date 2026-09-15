package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	bridge "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"

	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestWorkflowRejectsUntrustedOrMissingDelegationBeforeBusinessAccess(t *testing.T) {
	verifier, _ := delegatedsubject.NewVerifierFromOptions(&delegatedsubject.Options{Enabled: true, CurrentKey: "test-current-key", TTL: time.Minute})
	service := NewParticipantAIExplanationService(verifier)
	// Nil business dependencies deliberately panic if transport authentication is bypassed.
	service.Workflow = &bridge.Participant{}
	for _, ctx := range []context.Context{context.Background(), withMTLSWorkload(context.Background(), serviceidentity.CollectionServerCertificateCommonName)} {
		_, err := service.RequestAIWorkflow(ctx, &interpretationpb.RequestAIWorkflowRequest{TesteeId: 7, AssessmentId: 42, ReportId: 99, RequestId: "00000000-0000-4000-8000-000000000001"})
		if err == nil {
			t.Fatal("untrusted request accepted")
		}
	}
}

func TestWorkflowReadRejectsWrongDelegatedPurpose(t *testing.T) {
	options := &delegatedsubject.Options{Enabled: true, CurrentKey: "test-current-key", TTL: time.Minute}
	signer, _ := delegatedsubject.NewSignerFromOptions(options)
	verifier, _ := delegatedsubject.NewVerifierFromOptions(options)
	service := NewParticipantAIExplanationService(verifier)
	service.Workflow = &bridge.Participant{}
	for _, purpose := range []string{delegatedsubject.PurposeAIExplanationRequest, delegatedsubject.PurposeAIExplanationCapability} {
		raw, err := signer.Sign(delegatedsubject.SignInput{UserID: "42", TesteeID: 7, OrgID: 9, Purpose: purpose, TTL: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(delegatedsubject.MetadataKey, raw))
		ctx = withMTLSWorkload(ctx, serviceidentity.CollectionServerCertificateCommonName)
		_, err = service.GetAIWorkflow(ctx, &interpretationpb.GetAIWorkflowRequest{TesteeId: 7, AssessmentId: 42, RequestId: "00000000-0000-4000-8000-000000000001"})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("purpose %s: %v", purpose, err)
		}
	}
}

func TestWorkflowReadProjectionStates(t *testing.T) {
	pending, err := toProtoAIWorkflowResult("request", nil)
	if err != nil || pending.Status != "accepted" || pending.ContentJson != "" || pending.Version != 0 {
		t.Fatalf("pending: %v %v", pending, err)
	}
	failed, err := toProtoAIWorkflowResult("request", &bridge.Event{Status: "failed", Version: 4, FailureCode: "internal-provider-detail"})
	if err != nil || failed.Status != "failed" || failed.Version != 4 || failed.ContentJson != "" {
		t.Fatalf("failed: %v %v", failed, err)
	}
	_, err = toProtoAIWorkflowResult("request", &bridge.Event{Status: "completed", ArtifactJSON: "broken"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("corrupt result: %v", err)
	}
}

func TestWorkflowReadReturnsOnlyContentAndProvenance(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"
	content := `{"schema_version":"ai-explanation-output/v1","summary":"测试解读"}`
	digest := "sha256:" + strings.Repeat("a", 64)
	artifact := bridge.Artifact{ID: id, SessionID: id, RunID: id, EvidenceSetID: id, InvocationID: id, EvidenceFingerprint: strings.Repeat("a", 64), ProviderRequestID: "private-provider-request", ContentJSON: content, ContentFingerprint: fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(content))), InputFingerprint: digest, ProfileID: "profile", ProfileVersion: "v6", ProfileFingerprint: digest, PromptFingerprint: digest, RouteFingerprint: digest, OutputValidatorVersion: "v1", SafetyValidatorVersion: "v1", AssessmentID: "42", ReportID: "99", SourceVersion: "report-v1:101", SchemaVersion: "qs-ai-artifact/v1"}
	raw, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	result, err := toProtoAIWorkflowResult(id, &bridge.Event{SessionID: id, Status: "completed", Version: 5, ArtifactJSON: string(raw)})
	if err != nil {
		t.Fatal(err)
	}
	if result.ContentJson != content || result.ArtifactId != id || result.ReportId != "99" || result.SourceVersion != "report-v1:101" || result.Version != 5 {
		t.Fatalf("result=%v", result)
	}
	if strings.Contains(result.String(), "private-provider-request") {
		t.Fatal("internal provider identifier exposed")
	}
}

func TestWorkflowSourceRequiresCorrectDelegationWithLegacyServiceAbsent(t *testing.T) {
	options := &delegatedsubject.Options{Enabled: true, CurrentKey: "test-current-key", TTL: time.Minute}
	signer, _ := delegatedsubject.NewSignerFromOptions(options)
	verifier, _ := delegatedsubject.NewVerifierFromOptions(options)
	service := NewParticipantAIExplanationService(verifier)
	service.Workflow = &bridge.Participant{}
	for _, purpose := range []string{delegatedsubject.PurposeAIExplanationRequest, delegatedsubject.PurposeAIExplanationGet} {
		raw, _ := signer.Sign(delegatedsubject.SignInput{UserID: "42", TesteeID: 7, OrgID: 9, Purpose: purpose, TTL: time.Minute})
		ctx := withMTLSWorkload(metadata.NewIncomingContext(context.Background(), metadata.Pairs(delegatedsubject.MetadataKey, raw)), serviceidentity.CollectionServerCertificateCommonName)
		if _, err := service.GetAIWorkflowSource(ctx, &interpretationpb.GetAIWorkflowSourceRequest{TesteeId: 7, AssessmentId: 42}); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("wrong purpose accepted: %v", err)
		}
	}
	if _, err := service.GetAIWorkflowSource(context.Background(), &interpretationpb.GetAIWorkflowSourceRequest{TesteeId: 7, AssessmentId: 42}); err == nil {
		t.Fatal("missing workload/delegation accepted")
	}
	service.Workflow = nil
	if _, err := service.GetAIWorkflowSource(context.Background(), &interpretationpb.GetAIWorkflowSourceRequest{TesteeId: 7, AssessmentId: 42}); status.Code(err) != codes.Unavailable {
		t.Fatalf("disabled workflow: %v", err)
	}
}
