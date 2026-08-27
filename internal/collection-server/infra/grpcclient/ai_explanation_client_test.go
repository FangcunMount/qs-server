package grpcclient

import (
	"context"
	"errors"
	"testing"
	"time"

	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	aiport "github.com/FangcunMount/qs-server/internal/collection-server/port/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type participantAIExplanationServiceStub struct {
	t        *testing.T
	verifier *delegatedsubject.Verifier
}

func (s participantAIExplanationServiceStub) GetAIExplanationCapability(ctx context.Context, request *interpretationpb.GetAIExplanationCapabilityRequest, _ ...grpc.CallOption) (*interpretationpb.AIExplanationResponse, error) {
	s.verify(ctx, delegatedsubject.PurposeAIExplanationCapability, request.GetTesteeId())
	return &interpretationpb.AIExplanationResponse{Status: "available", SourceState: "current"}, nil
}

func (s participantAIExplanationServiceStub) RequestAIExplanation(ctx context.Context, request *interpretationpb.RequestAIExplanationRequest, _ ...grpc.CallOption) (*interpretationpb.AIExplanationResponse, error) {
	s.verify(ctx, delegatedsubject.PurposeAIExplanationRequest, request.GetTesteeId())
	return &interpretationpb.AIExplanationResponse{Status: "pending", GenerationId: "9001", SourceState: "current"}, nil
}

func (s participantAIExplanationServiceStub) GetAIExplanation(ctx context.Context, request *interpretationpb.GetAIExplanationRequest, _ ...grpc.CallOption) (*interpretationpb.AIExplanationResponse, error) {
	s.verify(ctx, delegatedsubject.PurposeAIExplanationGet, request.GetTesteeId())
	return &interpretationpb.AIExplanationResponse{
		Status:         "generated",
		GenerationId:   request.GetGenerationId(),
		ArtifactId:     "artifact-1",
		SourceReportId: "report-1",
		SourceState:    "current",
		Content: &interpretationpb.AIExplanationContent{
			SchemaVersion: "ai-explanation-output/v1",
			Summary:       "summary",
			IntegratedInsights: []*interpretationpb.AIExplanationIntegratedInsight{{
				Kind: "pattern", Title: "title", Content: "content", WhyItMatters: "why",
				EvidenceRefs: []*interpretationpb.AIExplanationEvidenceRef{{Kind: "dimension", Ref: "d1"}},
			}},
			Suggestions: []*interpretationpb.AIExplanationSuggestion{{
				Origin: "ai", Category: "action", Title: "suggestion", Goal: "goal",
				Actions: []string{"step"}, Rationale: "because",
				EvidenceRefs:         []*interpretationpb.AIExplanationEvidenceRef{{Kind: "insight", Ref: "i1"}},
				SourceSuggestionRefs: []string{"standard-1"}, Caution: "caution",
			}},
			Limitations: []string{"limited"},
		},
	}, nil
}

func (s participantAIExplanationServiceStub) ExportAIExplanations(ctx context.Context, request *interpretationpb.ExportAIExplanationsRequest, _ ...grpc.CallOption) (*interpretationpb.AIExplanationSubjectExportResponse, error) {
	s.verify(ctx, delegatedsubject.PurposeAIExplanationExport, request.GetTesteeId())
	return &interpretationpb.AIExplanationSubjectExportResponse{
		SchemaVersion: "ai-explanation-subject-export/v1", OrgId: 9, TesteeId: request.GetTesteeId(),
		ExportedAt: "2026-08-27T12:00:00Z", SnapshotAt: "2026-08-27T12:00:00Z", NextCursor: "next",
		Items: []*interpretationpb.AIExplanationSubjectExportItem{{
			GenerationId: "9001", ArtifactId: "9002", GeneratedAt: "2026-08-27T11:00:00Z",
			Source:  &interpretationpb.AIExplanationExportSourceReceipt{AssessmentId: "42", ReportId: "51", OutcomeId: "61", ReportType: "standard"},
			Release: &interpretationpb.AIExplanationExportReleaseReceipt{ProfileId: "profile", ProfileVersion: "v1", ProviderRoute: "route"},
			Content: &interpretationpb.AIExplanationContent{SchemaVersion: "ai-explanation-output/v1", Summary: "summary"},
		}},
	}, nil
}

func (s participantAIExplanationServiceStub) verify(ctx context.Context, purpose string, testeeID uint64) {
	s.t.Helper()
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		s.t.Fatal("delegated subject metadata missing")
	}
	values := md.Get(delegatedsubject.MetadataKey)
	if len(values) != 1 {
		s.t.Fatalf("delegated subject token count = %d", len(values))
	}
	token, err := s.verifier.Verify(values[0], purpose, testeeID)
	if err != nil {
		s.t.Fatalf("verify delegated subject for %q: %v", purpose, err)
	}
	if token.UserID != "42" || token.OrgID != 9 {
		s.t.Fatalf("delegated subject = %#v", token)
	}
}

func TestParticipantAIExplanationClientSignsEveryPurposeAndConvertsContent(t *testing.T) {
	options := &delegatedsubject.Options{Enabled: true, CurrentKey: "ai-client-test-key", TTL: time.Minute}
	signer, err := delegatedsubject.NewSignerFromOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := delegatedsubject.NewVerifierFromOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	client := &ParticipantAIExplanationClient{
		client:  &Client{config: &ClientConfig{Timeout: time.Second}},
		service: participantAIExplanationServiceStub{t: t, verifier: verifier},
		signer:  signer,
	}
	ctx := context.WithValue(context.Background(), pkgmiddleware.UserClaimsContextKey{}, &pkgmiddleware.UserClaims{UserID: "42", OrgID: "9"})

	if _, err := client.GetCapability(ctx, 7, 42, "zh-CN", []string{"work"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Request(ctx, 7, 42, "zh-CN", []string{"work"}); err != nil {
		t.Fatal(err)
	}
	result, err := client.Get(ctx, 7, 42, "9001")
	if err != nil {
		t.Fatal(err)
	}
	if result.Content == nil || result.Content.Summary != "summary" || len(result.Content.IntegratedInsights) != 1 || len(result.Content.Suggestions) != 1 {
		t.Fatalf("converted result = %#v", result)
	}
	if got := result.Content.Suggestions[0]; got.Caution != "caution" || len(got.SourceSuggestionRefs) != 1 || len(got.EvidenceRefs) != 1 {
		t.Fatalf("converted suggestion = %#v", got)
	}
	exported, err := client.Export(ctx, 7, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if exported.TesteeID != 7 || exported.OrgID != 9 || len(exported.Items) != 1 || exported.Items[0].Release.ProviderRoute != "route" || exported.NextCursor != "next" {
		t.Fatalf("converted export = %#v", exported)
	}
}

func TestConvertAIExplanationResponseMapsUnimplementedToDisabled(t *testing.T) {
	_, err := convertAIExplanationResponse(nil, status.Error(codes.Unimplemented, "not registered"))
	if !errors.Is(err, aiport.ErrDisabled) {
		t.Fatalf("error = %v", err)
	}
}
