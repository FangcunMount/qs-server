package service

import (
	"context"
	"errors"

	"strings"

	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	bridge "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ParticipantAIExplanationService struct {
	interpretationpb.UnimplementedParticipantAIExplanationServiceServer
	CurrentAccess *bridge.CurrentAccess
	Workflow      *bridge.Participant

	delegatedVerifier *delegatedsubject.Verifier
}

func NewParticipantAIExplanationService(verifier *delegatedsubject.Verifier) *ParticipantAIExplanationService {
	return &ParticipantAIExplanationService{delegatedVerifier: verifier}
}

func (s *ParticipantAIExplanationService) RegisterService(server *grpc.Server) {
	interpretationpb.RegisterParticipantAIExplanationServiceServer(server, s)
}

func toAIExplanationGRPCError(err error) error {
	mapped := toAssessmentQueryGRPCError(err)
	if status.Code(mapped) == codes.Internal {
		return status.Error(codes.Internal, "AI explanation request failed")
	}
	return mapped
}

func (s *ParticipantAIExplanationService) RequestAIWorkflow(ctx context.Context, request *interpretationpb.RequestAIWorkflowRequest) (*interpretationpb.AIWorkflowAccepted, error) {
	if request == nil || request.TesteeId == 0 || request.AssessmentId == 0 || request.ReportId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee, assessment and report are required")
	}
	if s.Workflow == nil {
		return nil, status.Error(codes.FailedPrecondition, "AI workflow is not configured")
	}
	actor, err := s.workflowActor(ctx, request.TesteeId, request.AssessmentId, delegatedsubject.PurposeAIExplanationRequest)
	if err != nil {
		return nil, err
	}
	err = s.Workflow.Request(ctx, actor, request.TesteeId, request.AssessmentId, request.ReportId, request.RequestId)
	if errors.Is(err, bridge.ErrInvalid) {
		return nil, status.Error(codes.InvalidArgument, "invalid workflow request")
	}
	if errors.Is(err, bridge.ErrConflict) {
		return nil, status.Error(codes.Aborted, "workflow request or source conflict")
	}
	if err != nil {
		return nil, toAIExplanationGRPCError(err)
	}
	return &interpretationpb.AIWorkflowAccepted{RequestId: request.RequestId, Status: "accepted"}, nil
}

// GetAIWorkflow rechecks current QS access; knowledge of a request ID grants no access.
func (s *ParticipantAIExplanationService) GetAIWorkflow(ctx context.Context, request *interpretationpb.GetAIWorkflowRequest) (*interpretationpb.AIWorkflowResult, error) {
	if request == nil || request.TesteeId == 0 || request.AssessmentId == 0 || strings.TrimSpace(request.RequestId) == "" {
		return nil, status.Error(codes.InvalidArgument, "testee, assessment and request are required")
	}
	if s.Workflow == nil {
		return nil, status.Error(codes.FailedPrecondition, "AI workflow is not configured")
	}
	actor, err := s.workflowActor(ctx, request.TesteeId, request.AssessmentId, delegatedsubject.PurposeAIExplanationGet)
	if err != nil {
		return nil, err
	}
	event, err := s.Workflow.Read(ctx, actor, request.TesteeId, request.AssessmentId, request.RequestId)
	if errors.Is(err, bridge.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "AI workflow not found")
	}
	if errors.Is(err, bridge.ErrInvalid) {
		return nil, status.Error(codes.InvalidArgument, "invalid workflow request")
	}
	if err != nil {
		return nil, toAIExplanationGRPCError(err)
	}
	return toProtoAIWorkflowResult(request.RequestId, event)
}

func toProtoAIWorkflowResult(requestID string, event *bridge.Event) (*interpretationpb.AIWorkflowResult, error) {
	result := &interpretationpb.AIWorkflowResult{RequestId: requestID, Status: "accepted"}
	if event == nil {
		return result, nil
	}
	result.Status, result.Version = event.Status, event.Version
	artifact, err := bridge.ValidateArtifact(*event)
	if err != nil {
		return nil, status.Error(codes.Internal, "invalid stored workflow result")
	}
	if artifact != nil {
		result.ContentJson, result.ArtifactId = artifact.ContentJSON, artifact.ID
		result.ReportId, result.SourceVersion = artifact.ReportID, artifact.SourceVersion
	}
	return result, nil
}

// GetAIWorkflowSource works independently of the retired legacy AI service.
func (s *ParticipantAIExplanationService) GetAIWorkflowSource(ctx context.Context, request *interpretationpb.GetAIWorkflowSourceRequest) (*interpretationpb.AIWorkflowSource, error) {
	if request == nil || request.TesteeId == 0 || request.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee and assessment are required")
	}
	if s.Workflow == nil {
		return nil, status.Error(codes.Unavailable, "AI workflow is not configured")
	}
	actor, err := s.workflowActor(ctx, request.TesteeId, request.AssessmentId, delegatedsubject.PurposeAIExplanationCapability)
	if err != nil {
		return nil, err
	}
	result, err := s.Workflow.Source(ctx, actor, request.TesteeId, request.AssessmentId)
	if errors.Is(err, bridge.ErrAccessDenied) {
		return nil, status.Error(codes.PermissionDenied, "AI eligibility access denied")
	}
	if errors.Is(err, bridge.ErrAccessUnavailable) {
		return nil, status.Error(codes.Unavailable, "AI eligibility temporarily unavailable")
	}
	if errors.Is(err, bridge.ErrInvalid) {
		return nil, status.Error(codes.InvalidArgument, "invalid workflow source request")
	}
	if err != nil {
		return nil, toAIExplanationGRPCError(err)
	}
	response := &interpretationpb.AIWorkflowSource{Status: result.Status, ReportId: result.ReportID, SourceVersion: result.SourceVersion}
	if result.AIEligibility != nil {
		response.AiEligibility = &interpretationpb.AIEligibility{Status: result.AIEligibility.Status, ReasonCode: result.AIEligibility.ReasonCode}
	}
	return response, nil
}

// workflowActor verifies the delegation before deriving organization context from
// the authorized QS testee. All participant workflow operations use this boundary.
func (s *ParticipantAIExplanationService) workflowActor(ctx context.Context, testeeID, assessmentID uint64, purpose string) (bridge.Actor, error) {
	token, err := verifyDelegatedSubject(ctx, s.delegatedVerifier, testeeID, purpose, true)
	if err != nil {
		return bridge.Actor{}, err
	}
	actor, err := s.CurrentAccess.ResolveParticipantActor(ctx, token.UserID, token.OrgID, testeeID, assessmentID)
	if err != nil {
		switch {
		case errors.Is(err, bridge.ErrInvalid):
			return bridge.Actor{}, status.Error(codes.InvalidArgument, "invalid workflow participant")
		case errors.Is(err, bridge.ErrAccessDenied):
			return bridge.Actor{}, status.Error(codes.PermissionDenied, "workflow access denied")
		case errors.Is(err, bridge.ErrAccessUnavailable):
			return bridge.Actor{}, status.Error(codes.Unavailable, "workflow authorization unavailable")
		default:
			return bridge.Actor{}, toAssessmentQueryGRPCError(err)
		}
	}
	return actor, nil
}
