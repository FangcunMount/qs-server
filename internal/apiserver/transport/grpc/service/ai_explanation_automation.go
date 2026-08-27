package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	aievaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	aiexecution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/execution"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AIExplanationAutomationService struct {
	interpretationpb.UnimplementedAIExplanationAutomationServiceServer
	executor         aiexecution.Executor
	evaluationRunner PromptEvaluationStepRunner
}

type PromptEvaluationStepRunner interface {
	RunStepV1(context.Context, aievaluation.OnlineStepCommand) (*aievaluation.OnlineStepResult, error)
}

func NewAIExplanationAutomationService(executor aiexecution.Executor, evaluationRunner PromptEvaluationStepRunner) *AIExplanationAutomationService {
	return &AIExplanationAutomationService{executor: executor, evaluationRunner: evaluationRunner}
}

func (s *AIExplanationAutomationService) ExecutePromptEvaluationStep(ctx context.Context, request *interpretationpb.ExecutePromptEvaluationStepRequest) (*interpretationpb.ExecutePromptEvaluationStepResponse, error) {
	if request == nil || request.GetOrgId() <= 0 || strings.TrimSpace(request.GetRunId()) == "" ||
		strings.TrimSpace(request.GetCaseId()) == "" || request.GetAttempt() < 1 ||
		strings.TrimSpace(request.GetRequestedBy()) == "" || strings.TrimSpace(request.GetEventId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "prompt evaluation step address and audit are required")
	}
	runID, err := meta.ParseID(request.GetRunId())
	if err != nil || runID.IsZero() {
		return nil, status.Error(codes.InvalidArgument, "run_id is invalid")
	}
	eventID := strings.TrimSpace(request.GetEventId())
	metadataEventID := strings.TrimSpace(interpretationTraceID(ctx))
	if metadataEventID == "" || metadataEventID != eventID {
		return nil, status.Error(codes.InvalidArgument, "event_id does not match event metadata")
	}
	if s.evaluationRunner == nil {
		return nil, status.Error(codes.FailedPrecondition, "AI explanation prompt evaluation is not configured")
	}
	result, err := s.evaluationRunner.RunStepV1(ctx, aievaluation.OnlineStepCommand{
		RunID: runID, CaseID: strings.TrimSpace(request.GetCaseId()), Attempt: int(request.GetAttempt()),
		Owner: eventID, RequestedOrgID: request.GetOrgId(), RequestedBy: strings.TrimSpace(request.GetRequestedBy()),
	})
	if err != nil {
		if errors.Is(err, aievaluation.ErrAttemptExecutionBusy) {
			return nil, status.Error(codes.Aborted, "prompt evaluation attempt is leased")
		}
		slog.ErrorContext(ctx, "AI explanation prompt evaluation step failed",
			slog.String("run_id", runID.String()), slog.String("case_id", request.GetCaseId()),
			slog.Int("attempt", int(request.GetAttempt())), slog.String("event_id", eventID), slog.String("error", err.Error()),
		)
		return nil, status.Error(codes.Internal, "AI explanation prompt evaluation step failed")
	}
	return mapPromptEvaluationStepResult(request, result)
}

func mapPromptEvaluationStepResult(request *interpretationpb.ExecutePromptEvaluationStepRequest, result *aievaluation.OnlineStepResult) (*interpretationpb.ExecutePromptEvaluationStepResponse, error) {
	if result == nil || result.Run == nil || result.Run.ID().String() != request.GetRunId() {
		return nil, status.Error(codes.Internal, "prompt evaluation step returned mismatched run")
	}
	switch result.Status {
	case aievaluation.OnlineStepProgressed, aievaluation.OnlineStepAlreadyCompleted, aievaluation.OnlineStepAwaitingReview, aievaluation.OnlineStepCanceled:
	default:
		return nil, status.Error(codes.Internal, "prompt evaluation step returned unsupported status")
	}
	if result.Status != aievaluation.OnlineStepCanceled && !result.Run.HasAttempt(request.GetCaseId(), int(request.GetAttempt())) {
		return nil, status.Error(codes.Internal, "prompt evaluation step did not persist its target")
	}
	response := &interpretationpb.ExecutePromptEvaluationStepResponse{
		Success: true, RunId: request.GetRunId(), CaseId: request.GetCaseId(), Attempt: request.GetAttempt(),
		Status: string(result.Status), RunStatus: string(result.Run.Status()),
	}
	if caseID, attempt, ok := result.Run.NextPendingGenerationAttempt(); ok {
		response.NextCaseId, response.NextAttempt = caseID, int32(attempt)
	}
	if result.Status == aievaluation.OnlineStepAwaitingReview && result.Run.Status() != domainevaluation.StatusAwaitingReview {
		return nil, status.Error(codes.Internal, "prompt evaluation step did not close collection")
	}
	if result.Status == aievaluation.OnlineStepCanceled && result.Run.Status() != domainevaluation.StatusCanceled {
		return nil, status.Error(codes.Internal, "prompt evaluation step did not preserve cancellation")
	}
	return response, nil
}

func (s *AIExplanationAutomationService) RegisterService(server *grpc.Server) {
	interpretationpb.RegisterAIExplanationAutomationServiceServer(server, s)
}

func (s *AIExplanationAutomationService) ExecuteAIExplanation(ctx context.Context, request *interpretationpb.ExecuteAIExplanationRequest) (*interpretationpb.ExecuteAIExplanationResponse, error) {
	if request == nil || strings.TrimSpace(request.GetGenerationId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "generation_id is required")
	}
	generationID, err := meta.ParseID(request.GetGenerationId())
	if err != nil || generationID.IsZero() {
		return nil, status.Error(codes.InvalidArgument, "generation_id is invalid")
	}
	traceID := strings.TrimSpace(request.GetTraceId())
	eventID := strings.TrimSpace(interpretationTraceID(ctx))
	requestEventID := strings.TrimSpace(request.GetEventId())
	if requestEventID == "" {
		requestEventID = eventID
	}
	if traceID == "" {
		traceID = eventID
	}
	if traceID == "" {
		return nil, status.Error(codes.InvalidArgument, "trace_id is required")
	}
	if eventID != "" && (eventID != traceID || eventID != requestEventID) {
		return nil, status.Error(codes.InvalidArgument, "trace_id does not match event metadata")
	}
	origin := retrygovernance.AttemptOrigin(strings.TrimSpace(request.GetAttemptOrigin()))
	hasRetryProof := request.GetExpectedAttempt() != 0 || origin != "" || strings.TrimSpace(request.GetActionRequestId()) != ""
	if hasRetryProof && (request.GetExpectedAttempt() < 1 || origin != retrygovernance.AttemptOriginManual ||
		strings.TrimSpace(request.GetActionRequestId()) == "" || requestEventID == "") {
		return nil, status.Error(codes.InvalidArgument, "AI explanation retry authorization is invalid")
	}
	expectedRunIDText := strings.TrimSpace(request.GetExpectedRunId())
	expectedLeaseText := strings.TrimSpace(request.GetExpectedLeaseExpiresAt())
	expectedPhase := domainrun.InvocationPhase(strings.TrimSpace(request.GetExpectedInvocationPhase()))
	hasRecoveryProof := expectedRunIDText != "" || expectedLeaseText != "" || expectedPhase != ""
	if hasRetryProof && hasRecoveryProof {
		return nil, status.Error(codes.InvalidArgument, "AI explanation retry and lease recovery proofs are mutually exclusive")
	}
	var expectedRunID meta.ID
	var expectedLeaseExpiresAt time.Time
	if hasRecoveryProof {
		expectedRunID, err = meta.ParseID(expectedRunIDText)
		if err != nil || expectedRunID.IsZero() {
			return nil, status.Error(codes.InvalidArgument, "AI explanation recovery run_id is invalid")
		}
		expectedLeaseExpiresAt, err = time.Parse(time.RFC3339Nano, expectedLeaseText)
		if err != nil || expectedLeaseExpiresAt.IsZero() ||
			(expectedPhase != domainrun.InvocationPhasePrepared && expectedPhase != domainrun.InvocationPhaseDispatching) || requestEventID == "" {
			return nil, status.Error(codes.InvalidArgument, "AI explanation lease recovery proof is invalid")
		}
	}
	if s.executor == nil {
		return nil, status.Error(codes.FailedPrecondition, "AI explanation automation is not configured")
	}

	result, err := s.executor.Execute(ctx, aiexecution.Command{
		GenerationID: generationID, TraceID: traceID, EventID: requestEventID,
		ExpectedAttempt: int(request.GetExpectedAttempt()), AttemptOrigin: origin,
		ActionRequestID: strings.TrimSpace(request.GetActionRequestId()),
		ExpectedRunID:   expectedRunID, ExpectedLeaseExpiresAt: expectedLeaseExpiresAt,
		ExpectedInvocationPhase: expectedPhase,
	})
	if err != nil {
		if errors.Is(err, domainrun.ErrRetryNotAllowed) || errors.Is(err, domainrun.ErrRecoveryNotAllowed) || errors.Is(err, domainrun.ErrConflict) {
			return nil, status.Error(codes.FailedPrecondition, "AI explanation recovery authorization is stale or invalid")
		}
		if errors.Is(err, domaingeneration.ErrOrgActiveCapacityExceeded) ||
			errors.Is(err, domaingeneration.ErrUserActiveCapacityExceeded) ||
			errors.Is(err, domaingeneration.ErrAssessmentActiveCapacityExceeded) {
			return nil, status.Error(codes.ResourceExhausted, "AI explanation execution capacity is temporarily exhausted")
		}
		slog.ErrorContext(ctx, "AI explanation automation execution failed",
			slog.String("generation_id", generationID.String()), slog.String("trace_id", traceID), slog.String("error", err.Error()),
		)
		return nil, status.Error(codes.Internal, "AI explanation execution failed")
	}
	response, err := mapAIExplanationExecutionResult(generationID, result)
	if err != nil {
		slog.ErrorContext(ctx, "AI explanation automation returned invalid application result",
			slog.String("generation_id", generationID.String()), slog.String("trace_id", traceID), slog.String("error", err.Error()),
		)
		return nil, status.Error(codes.Internal, "AI explanation execution result is invalid")
	}
	return response, nil
}

func mapAIExplanationExecutionResult(expectedGenerationID meta.ID, result *aiexecution.Result) (*interpretationpb.ExecuteAIExplanationResponse, error) {
	if result == nil || result.Generation == nil || result.Generation.ID() != expectedGenerationID {
		return nil, fmt.Errorf("AI explanation Generation result does not match request")
	}
	response := &interpretationpb.ExecuteAIExplanationResponse{
		Status:       string(result.Status),
		GenerationId: result.Generation.ID().String(),
	}
	if result.Run != nil {
		if result.Run.GenerationID() != expectedGenerationID {
			return nil, fmt.Errorf("AI explanation Run result does not match Generation")
		}
		response.RunId = result.Run.ID().String()
	}
	switch result.Status {
	case aiexecution.StatusGenerated:
		if result.Artifact == nil || result.Artifact.GenerationID() != expectedGenerationID {
			return nil, fmt.Errorf("generated AI explanation Artifact is required")
		}
		response.Success = true
		response.ArtifactId = result.Artifact.ID().String()
	case aiexecution.StatusProcessing:
		if result.Run == nil {
			return nil, fmt.Errorf("processing AI explanation Run is required")
		}
		response.Success = true
	case aiexecution.StatusFailed:
		failure := result.Failure
		if failure == nil && result.Run != nil {
			failure = result.Run.Failure()
		}
		if result.Run == nil || failure == nil || strings.TrimSpace(failure.Code) == "" || strings.TrimSpace(failure.SafeMessage) == "" {
			return nil, fmt.Errorf("failed AI explanation result is incomplete")
		}
		response.FailureKind = string(failure.Kind)
		response.FailureCode = failure.Code
		response.SafeMessage = failure.SafeMessage
		response.Retryable = failure.Retryable
	default:
		return nil, fmt.Errorf("unsupported AI explanation execution status %q", result.Status)
	}
	return response, nil
}
