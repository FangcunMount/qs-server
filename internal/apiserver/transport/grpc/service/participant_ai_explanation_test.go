package service

import (
	"context"
	"testing"
	"time"

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
