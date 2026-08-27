package grpcclient

import (
	"context"
	"fmt"

	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	aiport "github.com/FangcunMount/qs-server/internal/collection-server/port/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ParticipantAIExplanationClient struct {
	client  *Client
	service interpretationpb.ParticipantAIExplanationServiceClient
	signer  *delegatedsubject.Signer
}

func NewParticipantAIExplanationClient(client *Client, signer *delegatedsubject.Signer) *ParticipantAIExplanationClient {
	return &ParticipantAIExplanationClient{
		client: client, service: interpretationpb.NewParticipantAIExplanationServiceClient(client.Conn()), signer: signer,
	}
}

func (c *ParticipantAIExplanationClient) GetCapability(ctx context.Context, testeeID, assessmentID uint64, locale string, focusAreas []string) (*aiport.Output, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	ctx, err := c.attachDelegatedSubject(ctx, testeeID, delegatedsubject.PurposeAIExplanationCapability)
	if err != nil {
		return nil, err
	}
	response, err := c.service.GetAIExplanationCapability(ctx, &interpretationpb.GetAIExplanationCapabilityRequest{
		AssessmentId: assessmentID, TesteeId: testeeID, Locale: locale, FocusAreas: append([]string(nil), focusAreas...),
	})
	return convertAIExplanationResponse(response, err)
}

func (c *ParticipantAIExplanationClient) Request(ctx context.Context, testeeID, assessmentID uint64, locale string, focusAreas []string) (*aiport.Output, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	ctx, err := c.attachDelegatedSubject(ctx, testeeID, delegatedsubject.PurposeAIExplanationRequest)
	if err != nil {
		return nil, err
	}
	response, err := c.service.RequestAIExplanation(ctx, &interpretationpb.RequestAIExplanationRequest{
		AssessmentId: assessmentID, TesteeId: testeeID, Locale: locale, FocusAreas: append([]string(nil), focusAreas...),
	})
	return convertAIExplanationResponse(response, err)
}

func (c *ParticipantAIExplanationClient) Get(ctx context.Context, testeeID, assessmentID uint64, generationID string) (*aiport.Output, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	ctx, err := c.attachDelegatedSubject(ctx, testeeID, delegatedsubject.PurposeAIExplanationGet)
	if err != nil {
		return nil, err
	}
	response, err := c.service.GetAIExplanation(ctx, &interpretationpb.GetAIExplanationRequest{
		AssessmentId: assessmentID, TesteeId: testeeID, GenerationId: generationID,
	})
	return convertAIExplanationResponse(response, err)
}

func (c *ParticipantAIExplanationClient) Export(ctx context.Context, testeeID uint64, pageSize int, cursor string) (*aiport.ExportPage, error) {
	if pageSize < 0 || pageSize > 100 {
		return nil, fmt.Errorf("AI explanation export page size is invalid")
	}
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()
	ctx, err := c.attachDelegatedSubject(ctx, testeeID, delegatedsubject.PurposeAIExplanationExport)
	if err != nil {
		return nil, err
	}
	response, err := c.service.ExportAIExplanations(ctx, &interpretationpb.ExportAIExplanationsRequest{
		TesteeId: testeeID, PageSize: int32(pageSize), Cursor: cursor,
	})
	return convertAIExplanationExportResponse(response, err)
}

func convertAIExplanationResponse(response *interpretationpb.AIExplanationResponse, err error) (*aiport.Output, error) {
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return nil, aiport.ErrDisabled
		}
		return nil, err
	}
	if response == nil {
		return nil, nil
	}
	result := &aiport.Output{
		Status: response.GetStatus(), ReasonCode: response.GetReasonCode(), GenerationID: response.GetGenerationId(),
		ArtifactID: response.GetArtifactId(), SourceReportID: response.GetSourceReportId(), SourceState: response.GetSourceState(),
		CreatedAt: response.GetCreatedAt(), UpdatedAt: response.GetUpdatedAt(),
	}
	if failure := response.GetFailure(); failure != nil {
		result.Failure = &aiport.Failure{Code: failure.GetCode(), SafeMessage: failure.GetSafeMessage(), Retryable: failure.GetRetryable()}
	}
	if content := response.GetContent(); content != nil {
		result.Content = convertAIExplanationContent(content)
	}
	return result, nil
}

func convertAIExplanationExportResponse(response *interpretationpb.AIExplanationSubjectExportResponse, err error) (*aiport.ExportPage, error) {
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return nil, aiport.ErrDisabled
		}
		return nil, err
	}
	if response == nil {
		return nil, nil
	}
	result := &aiport.ExportPage{
		SchemaVersion: response.GetSchemaVersion(), OrgID: response.GetOrgId(), TesteeID: response.GetTesteeId(),
		ExportedAt: response.GetExportedAt(), SnapshotAt: response.GetSnapshotAt(),
		Items: make([]aiport.ExportItem, 0, len(response.GetItems())), NextCursor: response.GetNextCursor(),
	}
	for _, item := range response.GetItems() {
		if item == nil || item.GetSource() == nil || item.GetRelease() == nil || item.GetContent() == nil {
			return nil, fmt.Errorf("AI explanation export contains an incomplete item")
		}
		source := item.GetSource()
		release := item.GetRelease()
		content := convertAIExplanationContent(item.GetContent())
		result.Items = append(result.Items, aiport.ExportItem{
			GenerationID: item.GetGenerationId(), ArtifactID: item.GetArtifactId(), GeneratedAt: item.GetGeneratedAt(),
			Source: aiport.ExportSourceReceipt{
				AssessmentID: source.GetAssessmentId(), ReportID: source.GetReportId(), OutcomeID: source.GetOutcomeId(), ReportType: source.GetReportType(),
				TemplateVersion: source.GetTemplateVersion(), ContentSchemaVersion: source.GetContentSchemaVersion(),
				BuilderIdentity: source.GetBuilderIdentity(), ReportGeneratedAt: source.GetReportGeneratedAt(),
			},
			Release: aiport.ExportReleaseReceipt{
				ProfileID: release.GetProfileId(), ProfileVersion: release.GetProfileVersion(), ProfileFingerprint: release.GetProfileFingerprint(),
				PromptTemplateID: release.GetPromptTemplateId(), PromptVersion: release.GetPromptVersion(), PromptFingerprint: release.GetPromptFingerprint(),
				PromptGitBlobSHA: release.GetPromptGitBlobSha(), ProviderRoute: release.GetProviderRoute(), ProviderRouteRevision: release.GetProviderRouteRevision(),
				ResolvedProvider: release.GetResolvedProvider(), ResolvedModel: release.GetResolvedModel(), ExecutionSpecFingerprint: release.GetExecutionSpecFingerprint(),
				InputSchema: release.GetInputSchema(), OutputSchema: release.GetOutputSchema(), SafetyPolicy: release.GetSafetyPolicy(),
				SchemaValidatorVersion: release.GetSchemaValidatorVersion(), ReferenceValidatorVersion: release.GetReferenceValidatorVersion(),
				ProfileValidatorVersion: release.GetProfileValidatorVersion(), SafetyValidatorVersion: release.GetSafetyValidatorVersion(), ValidatedAt: release.GetValidatedAt(),
			},
			Content: *content,
		})
	}
	return result, nil
}

func convertAIExplanationContent(content *interpretationpb.AIExplanationContent) *aiport.Content {
	result := &aiport.Content{
		SchemaVersion: content.GetSchemaVersion(), Summary: content.GetSummary(), Limitations: append([]string(nil), content.GetLimitations()...),
	}
	for _, insight := range content.GetIntegratedInsights() {
		if insight == nil {
			continue
		}
		result.IntegratedInsights = append(result.IntegratedInsights, aiport.IntegratedInsight{
			Kind: insight.GetKind(), Title: insight.GetTitle(), Content: insight.GetContent(), WhyItMatters: insight.GetWhyItMatters(),
			EvidenceRefs: convertAIExplanationEvidenceRefs(insight.GetEvidenceRefs()),
		})
	}
	for _, suggestion := range content.GetSuggestions() {
		if suggestion == nil {
			continue
		}
		result.Suggestions = append(result.Suggestions, aiport.Suggestion{
			Origin: suggestion.GetOrigin(), Category: suggestion.GetCategory(), Title: suggestion.GetTitle(), Goal: suggestion.GetGoal(),
			Actions: append([]string(nil), suggestion.GetActions()...), Rationale: suggestion.GetRationale(),
			EvidenceRefs:         convertAIExplanationEvidenceRefs(suggestion.GetEvidenceRefs()),
			SourceSuggestionRefs: append([]string(nil), suggestion.GetSourceSuggestionRefs()...), Caution: suggestion.GetCaution(),
		})
	}
	return result
}

func convertAIExplanationEvidenceRefs(refs []*interpretationpb.AIExplanationEvidenceRef) []aiport.EvidenceRef {
	result := make([]aiport.EvidenceRef, 0, len(refs))
	for _, ref := range refs {
		if ref != nil {
			result = append(result, aiport.EvidenceRef{Kind: ref.GetKind(), Ref: ref.GetRef()})
		}
	}
	return result
}

func (c *ParticipantAIExplanationClient) attachDelegatedSubject(ctx context.Context, testeeID uint64, purpose string) (context.Context, error) {
	if c == nil || c.signer == nil || !c.signer.Enabled() {
		return ctx, nil
	}
	input, err := delegatedsubject.SignInputFromContext(ctx, testeeID, purpose, 0)
	if err != nil {
		return ctx, err
	}
	return delegatedsubject.AppendToOutgoingContext(ctx, c.signer, input)
}
