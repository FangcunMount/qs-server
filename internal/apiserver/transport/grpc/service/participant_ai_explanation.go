package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	aiparticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/participant"
	aisubjectexport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/subjectexport"
	domainartifact "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainoutput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ParticipantAIExplanationService struct {
	interpretationpb.UnimplementedParticipantAIExplanationServiceServer
	service           aiparticipant.Service
	subjectExport     *aisubjectexport.Service
	delegatedVerifier *delegatedsubject.Verifier
}

func NewParticipantAIExplanationService(service aiparticipant.Service, subjectExport *aisubjectexport.Service, verifier *delegatedsubject.Verifier) *ParticipantAIExplanationService {
	return &ParticipantAIExplanationService{service: service, subjectExport: subjectExport, delegatedVerifier: verifier}
}

func (s *ParticipantAIExplanationService) RegisterService(server *grpc.Server) {
	interpretationpb.RegisterParticipantAIExplanationServiceServer(server, s)
}

func (s *ParticipantAIExplanationService) GetAIExplanationCapability(ctx context.Context, request *interpretationpb.GetAIExplanationCapabilityRequest) (*interpretationpb.AIExplanationResponse, error) {
	if request == nil || request.GetAssessmentId() == 0 || request.GetTesteeId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "assessment_id and testee_id are required")
	}
	actor, err := s.authorize(ctx, request.GetTesteeId(), delegatedsubject.PurposeAIExplanationCapability)
	if err != nil {
		return nil, err
	}
	result, err := s.service.Capability(ctx, actor, aiparticipant.RequestInput{
		AssessmentID: meta.FromUint64(request.GetAssessmentId()), Locale: strings.TrimSpace(request.GetLocale()), FocusAreas: request.GetFocusAreas(),
	})
	if err != nil {
		return nil, toAIExplanationGRPCError(err)
	}
	return toProtoAIExplanationResult(result), nil
}

func (s *ParticipantAIExplanationService) RequestAIExplanation(ctx context.Context, request *interpretationpb.RequestAIExplanationRequest) (*interpretationpb.AIExplanationResponse, error) {
	if request == nil || request.GetAssessmentId() == 0 || request.GetTesteeId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "assessment_id and testee_id are required")
	}
	actor, err := s.authorize(ctx, request.GetTesteeId(), delegatedsubject.PurposeAIExplanationRequest)
	if err != nil {
		return nil, err
	}
	result, err := s.service.Request(ctx, actor, aiparticipant.RequestInput{
		AssessmentID: meta.FromUint64(request.GetAssessmentId()), Locale: strings.TrimSpace(request.GetLocale()), FocusAreas: request.GetFocusAreas(),
	})
	if err != nil {
		return nil, toAIExplanationGRPCError(err)
	}
	return toProtoAIExplanationResult(result), nil
}

func (s *ParticipantAIExplanationService) GetAIExplanation(ctx context.Context, request *interpretationpb.GetAIExplanationRequest) (*interpretationpb.AIExplanationResponse, error) {
	if request == nil || request.GetAssessmentId() == 0 || request.GetTesteeId() == 0 || strings.TrimSpace(request.GetGenerationId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "assessment_id, testee_id and generation_id are required")
	}
	generationID, err := meta.ParseID(request.GetGenerationId())
	if err != nil || generationID.IsZero() {
		return nil, status.Error(codes.InvalidArgument, "generation_id is invalid")
	}
	actor, err := s.authorize(ctx, request.GetTesteeId(), delegatedsubject.PurposeAIExplanationGet)
	if err != nil {
		return nil, err
	}
	result, err := s.service.Get(ctx, actor, aiparticipant.GetInput{AssessmentID: meta.FromUint64(request.GetAssessmentId()), GenerationID: generationID})
	if err != nil {
		return nil, toAIExplanationGRPCError(err)
	}
	return toProtoAIExplanationResult(result), nil
}

func (s *ParticipantAIExplanationService) ExportAIExplanations(ctx context.Context, request *interpretationpb.ExportAIExplanationsRequest) (*interpretationpb.AIExplanationSubjectExportResponse, error) {
	if request == nil || request.GetTesteeId() == 0 || request.GetPageSize() < 0 || request.GetPageSize() > aisubjectexport.MaxPageSize {
		return nil, status.Error(codes.InvalidArgument, "testee_id and a valid page_size are required")
	}
	if s == nil || s.subjectExport == nil {
		return nil, status.Error(codes.FailedPrecondition, "participant AI explanation export is not configured")
	}
	token, err := verifyDelegatedSubject(ctx, s.delegatedVerifier, request.GetTesteeId(), delegatedsubject.PurposeAIExplanationExport, true)
	if err != nil {
		return nil, err
	}
	if token.OrgID == 0 || token.OrgID > math.MaxInt64 {
		return nil, status.Error(codes.PermissionDenied, "participant AI explanation export organization is invalid")
	}
	page, err := s.subjectExport.Export(ctx, aisubjectexport.Query{
		Subject:  aisubjectexport.Subject{OrgID: int64(token.OrgID), TesteeID: meta.FromUint64(token.TesteeID)},
		PageSize: int(request.GetPageSize()), Cursor: strings.TrimSpace(request.GetCursor()),
	})
	if err != nil {
		if errors.Is(err, aisubjectexport.ErrInvalidQuery) {
			return nil, status.Error(codes.InvalidArgument, "AI explanation export query is invalid")
		}
		return nil, status.Error(codes.Internal, "AI explanation export failed")
	}
	return toProtoAIExplanationSubjectExport(page), nil
}

func (s *ParticipantAIExplanationService) authorize(ctx context.Context, testeeID uint64, purpose string) (aiparticipant.Actor, error) {
	if s == nil || s.service == nil {
		return aiparticipant.Actor{}, status.Error(codes.FailedPrecondition, "participant AI explanation is not configured")
	}
	token, err := verifyDelegatedSubject(ctx, s.delegatedVerifier, testeeID, purpose, true)
	if err != nil {
		return aiparticipant.Actor{}, err
	}
	return aiparticipant.Actor{SubjectID: token.UserID, TesteeID: token.TesteeID}, nil
}

func toAIExplanationGRPCError(err error) error {
	switch {
	case errors.Is(err, aiparticipant.ErrInvalidRequest):
		return status.Error(codes.InvalidArgument, "AI explanation request is invalid")
	case errors.Is(err, aiparticipant.ErrAccessMismatch):
		return status.Error(codes.PermissionDenied, "AI explanation does not belong to participant assessment")
	case errors.Is(err, aiparticipant.ErrConfiguration):
		return status.Error(codes.FailedPrecondition, "AI explanation is not configured")
	case errors.Is(err, domaingeneration.ErrOrgDailyBudgetExceeded),
		errors.Is(err, domaingeneration.ErrUserDailyBudgetExceeded),
		errors.Is(err, domaingeneration.ErrAssessmentDailyBudgetExceeded):
		return status.Error(codes.ResourceExhausted, "AI explanation daily capacity exceeded")
	case errors.Is(err, domaingeneration.ErrNotFound), errors.Is(err, domainrun.ErrNotFound), errors.Is(err, domainartifact.ErrNotFound):
		return status.Error(codes.NotFound, "AI explanation not found")
	default:
		mapped := toAssessmentQueryGRPCError(err)
		if status.Code(mapped) == codes.Internal {
			return status.Error(codes.Internal, "AI explanation request failed")
		}
		return mapped
	}
}

func toProtoAIExplanationResult(result *aiparticipant.Result) *interpretationpb.AIExplanationResponse {
	if result == nil {
		return nil
	}
	response := &interpretationpb.AIExplanationResponse{
		Status: string(result.Status), ReasonCode: result.ReasonCode, SourceState: string(result.SourceState),
		GenerationId: optionalAIExplanationID(result.GenerationID), ArtifactId: optionalAIExplanationID(result.ArtifactID),
		SourceReportId: optionalAIExplanationID(result.SourceReportID), CreatedAt: optionalAIExplanationTime(result.CreatedAt), UpdatedAt: optionalAIExplanationTime(result.UpdatedAt),
	}
	if result.Content != nil {
		response.Content = toProtoAIExplanationContent(*result.Content)
	}
	if result.Failure != nil {
		response.Failure = &interpretationpb.AIExplanationFailure{Code: result.Failure.Code, SafeMessage: result.Failure.SafeMessage, Retryable: result.Failure.Retryable}
	}
	return response
}

func toProtoAIExplanationSubjectExport(page *aisubjectexport.Page) *interpretationpb.AIExplanationSubjectExportResponse {
	if page == nil {
		return nil
	}
	response := &interpretationpb.AIExplanationSubjectExportResponse{
		SchemaVersion: page.SchemaVersion, OrgId: uint64(page.Subject.OrgID), TesteeId: page.Subject.TesteeID.Uint64(),
		ExportedAt: optionalAIExplanationTime(page.ExportedAt), SnapshotAt: optionalAIExplanationTime(page.SnapshotAt),
		Items: make([]*interpretationpb.AIExplanationSubjectExportItem, 0, len(page.Items)), NextCursor: page.NextCursor,
	}
	for _, item := range page.Items {
		response.Items = append(response.Items, &interpretationpb.AIExplanationSubjectExportItem{
			GenerationId: item.GenerationID.String(), ArtifactId: item.ArtifactID.String(), GeneratedAt: optionalAIExplanationTime(item.GeneratedAt),
			Source: &interpretationpb.AIExplanationExportSourceReceipt{
				AssessmentId: item.Source.AssessmentID.String(), ReportId: item.Source.ReportID.String(), OutcomeId: item.Source.OutcomeID.String(),
				ReportType: item.Source.ReportType, TemplateVersion: item.Source.TemplateVersion, ContentSchemaVersion: item.Source.ContentSchemaVersion,
				BuilderIdentity: item.Source.BuilderIdentity, ReportGeneratedAt: optionalAIExplanationTime(item.Source.ReportGeneratedAt),
			},
			Release: &interpretationpb.AIExplanationExportReleaseReceipt{
				ProfileId: item.Release.ProfileID, ProfileVersion: item.Release.ProfileVersion, ProfileFingerprint: item.Release.ProfileFingerprint,
				PromptTemplateId: item.Release.PromptTemplateID, PromptVersion: item.Release.PromptVersion, PromptFingerprint: item.Release.PromptFingerprint,
				PromptGitBlobSha: item.Release.PromptGitBlobSHA, ProviderRoute: item.Release.ProviderRoute,
				ProviderRouteRevision: item.Release.ProviderRouteRevision, ResolvedProvider: item.Release.ResolvedProvider, ResolvedModel: item.Release.ResolvedModel,
				ExecutionSpecFingerprint: item.Release.ExecutionSpecFingerprint, InputSchema: item.Release.InputSchema, OutputSchema: item.Release.OutputSchema,
				SafetyPolicy: item.Release.SafetyPolicy, SchemaValidatorVersion: item.Release.SchemaValidatorVersion,
				ReferenceValidatorVersion: item.Release.ReferenceValidatorVersion, ProfileValidatorVersion: item.Release.ProfileValidatorVersion,
				SafetyValidatorVersion: item.Release.SafetyValidatorVersion, ValidatedAt: optionalAIExplanationTime(item.Release.ValidatedAt),
			},
			Content: toProtoAIExplanationContent(item.Content),
		})
	}
	return response
}

func toProtoAIExplanationContent(content domainoutput.Content) *interpretationpb.AIExplanationContent {
	insights := make([]*interpretationpb.AIExplanationIntegratedInsight, 0, len(content.IntegratedInsights))
	for _, insight := range content.IntegratedInsights {
		insights = append(insights, &interpretationpb.AIExplanationIntegratedInsight{
			Kind: string(insight.Kind), Title: insight.Title, Content: insight.Content, WhyItMatters: insight.WhyItMatters,
			EvidenceRefs: toProtoAIExplanationEvidenceRefs(insight.EvidenceRefs),
		})
	}
	suggestions := make([]*interpretationpb.AIExplanationSuggestion, 0, len(content.Suggestions))
	for _, suggestion := range content.Suggestions {
		suggestions = append(suggestions, &interpretationpb.AIExplanationSuggestion{
			Origin: string(suggestion.Origin), Category: suggestion.Category, Title: suggestion.Title, Goal: suggestion.Goal,
			Actions: append([]string(nil), suggestion.Actions...), Rationale: suggestion.Rationale,
			EvidenceRefs:         toProtoAIExplanationEvidenceRefs(suggestion.EvidenceRefs),
			SourceSuggestionRefs: append([]string(nil), suggestion.SourceSuggestionRefs...), Caution: suggestion.Caution,
		})
	}
	return &interpretationpb.AIExplanationContent{
		SchemaVersion: content.SchemaVersion, Summary: content.Summary, IntegratedInsights: insights,
		Suggestions: suggestions, Limitations: append([]string(nil), content.Limitations...),
	}
}

func toProtoAIExplanationEvidenceRefs(refs []domainoutput.EvidenceRef) []*interpretationpb.AIExplanationEvidenceRef {
	result := make([]*interpretationpb.AIExplanationEvidenceRef, 0, len(refs))
	for _, ref := range refs {
		result = append(result, &interpretationpb.AIExplanationEvidenceRef{Kind: string(ref.Kind), Ref: ref.Ref})
	}
	return result
}

func optionalAIExplanationID(id meta.ID) string {
	if id.IsZero() {
		return ""
	}
	return id.String()
}

func optionalAIExplanationTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
