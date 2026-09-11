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
	aiparticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/participant"
	aisubjectexport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/subjectexport"
	domainartifact "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type grpcSubjectExportReader struct {
	query aisubjectexport.ReadQuery
}

func TestParticipantAIExplanationInvalidInputMapsToInvalidArgument(t *testing.T) {
	mapped := toAIExplanationGRPCError(aiparticipant.ErrInvalidRequest)
	if status.Code(mapped) != codes.InvalidArgument {
		t.Fatalf("mapped error = %v", mapped)
	}
}

func (r *grpcSubjectExportReader) ListParticipantArtifacts(_ context.Context, query aisubjectexport.ReadQuery) ([]*domainartifact.AIExplanationArtifact, error) {
	r.query = query
	return []*domainartifact.AIExplanationArtifact{}, nil
}

func TestParticipantAIExplanationCapacityMapsToResourceExhausted(t *testing.T) {
	for _, err := range []error{
		domaingeneration.ErrOrgDailyBudgetExceeded,
		domaingeneration.ErrUserDailyBudgetExceeded,
		domaingeneration.ErrAssessmentDailyBudgetExceeded,
	} {
		mapped := toAIExplanationGRPCError(err)
		if status.Code(mapped) != codes.ResourceExhausted || status.Convert(mapped).Message() != "AI explanation daily capacity exceeded" {
			t.Fatalf("mapped error = %v", mapped)
		}
	}
}

func TestParticipantAIExplanationExportUsesDelegatedOrganization(t *testing.T) {
	reader := &grpcSubjectExportReader{}
	exporter, err := aisubjectexport.NewService(reader, func() time.Time {
		return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	options := &delegatedsubject.Options{Enabled: true, CurrentKey: "test-current-key", TTL: time.Minute}
	signer, err := delegatedsubject.NewSignerFromOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := delegatedsubject.NewVerifierFromOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := signer.Sign(delegatedsubject.SignInput{
		UserID: "42", TesteeID: 7, OrgID: 9, Purpose: delegatedsubject.PurposeAIExplanationExport, TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(delegatedsubject.MetadataKey, raw))
	ctx = withMTLSWorkload(ctx, serviceidentity.CollectionServerCertificateCommonName)
	service := NewParticipantAIExplanationService(nil, exporter, verifier)
	response, err := service.ExportAIExplanations(ctx, &interpretationpb.ExportAIExplanationsRequest{TesteeId: 7, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if response.GetSchemaVersion() != aisubjectexport.SchemaVersionV1 || response.GetOrgId() != 9 || response.GetTesteeId() != 7 || len(response.GetItems()) != 0 {
		t.Fatalf("export response = %#v", response)
	}
	if reader.query.Subject.OrgID != 9 || reader.query.Subject.TesteeID.Uint64() != 7 || reader.query.Limit != 26 {
		t.Fatalf("export read query = %#v", reader.query)
	}
}

func TestParticipantAIExplanationExportRejectsMissingOrganization(t *testing.T) {
	reader := &grpcSubjectExportReader{}
	exporter, _ := aisubjectexport.NewService(reader, time.Now)
	options := &delegatedsubject.Options{Enabled: true, CurrentKey: "test-current-key", TTL: time.Minute}
	signer, _ := delegatedsubject.NewSignerFromOptions(options)
	verifier, _ := delegatedsubject.NewVerifierFromOptions(options)
	raw, _ := signer.Sign(delegatedsubject.SignInput{UserID: "42", TesteeID: 7, Purpose: delegatedsubject.PurposeAIExplanationExport, TTL: time.Minute})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(delegatedsubject.MetadataKey, raw))
	ctx = withMTLSWorkload(ctx, serviceidentity.CollectionServerCertificateCommonName)
	_, err := NewParticipantAIExplanationService(nil, exporter, verifier).ExportAIExplanations(ctx, &interpretationpb.ExportAIExplanationsRequest{TesteeId: 7})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestWorkflowRejectsUntrustedOrMissingDelegationBeforeBusinessAccess(t *testing.T) {
	verifier, _ := delegatedsubject.NewVerifierFromOptions(&delegatedsubject.Options{Enabled: true, CurrentKey: "test-current-key", TTL: time.Minute})
	service := NewParticipantAIExplanationService(nil, nil, verifier)
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
	service := NewParticipantAIExplanationService(nil, nil, verifier)
	service.Workflow = &bridge.Participant{}
	for _, purpose := range []string{delegatedsubject.PurposeAIExplanationRequest, delegatedsubject.PurposeAIExplanationExport} {
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
