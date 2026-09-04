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
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
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
	RunRecheckV1(context.Context, aievaluation.RunRecheckCommand) (*aievaluation.OnlineRecheckResult, error)
	RunStepV2(context.Context, aievaluation.OnlineStepV2Command) (*aievaluation.OnlineStepV2Result, error)
}

func NewAIExplanationAutomationService(executor aiexecution.Executor, evaluationRunner PromptEvaluationStepRunner) *AIExplanationAutomationService {
	return &AIExplanationAutomationService{executor: executor, evaluationRunner: evaluationRunner}
}

func (s *AIExplanationAutomationService) ExecutePromptEvaluationStep(ctx context.Context, request *interpretationpb.ExecutePromptEvaluationStepRequest) (*interpretationpb.ExecutePromptEvaluationStepResponse, error) {
	if err := validatePromptEvaluationStepRequest(request); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
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
	if strings.TrimSpace(request.GetEvidenceVersion()) == eventpayload.AIExplanationPromptEvaluationEvidenceVersionV2 {
		return s.executePromptEvaluationStepV2(ctx, request, runID, eventID)
	}
	if recheckIDText := strings.TrimSpace(request.GetRecheckId()); recheckIDText != "" {
		recheckID, parseErr := meta.ParseID(recheckIDText)
		if parseErr != nil || recheckID.IsZero() {
			return nil, status.Error(codes.InvalidArgument, "recheck_id is invalid")
		}
		result, runErr := s.evaluationRunner.RunRecheckV1(ctx, aievaluation.RunRecheckCommand{
			RecheckID: recheckID, SourceRunID: runID, CaseID: strings.TrimSpace(request.GetCaseId()), Attempt: int(request.GetAttempt()),
			Owner: eventID, RequestedOrg: request.GetOrgId(), RequestedBy: strings.TrimSpace(request.GetRequestedBy()),
		})
		if runErr != nil {
			if errors.Is(runErr, aievaluation.ErrAttemptExecutionBusy) {
				return nil, status.Error(codes.Aborted, "prompt evaluation recheck is leased")
			}
			slog.ErrorContext(ctx, "AI explanation prompt evaluation recheck failed",
				slog.String("run_id", runID.String()), slog.String("recheck_id", recheckID.String()),
				slog.String("case_id", request.GetCaseId()), slog.Int("attempt", int(request.GetAttempt())),
				slog.String("event_id", eventID), slog.String("error", runErr.Error()),
			)
			return nil, status.Error(codes.Internal, "AI explanation prompt evaluation recheck failed")
		}
		return mapPromptEvaluationRecheckResult(request, result)
	}
	return nil, status.Error(codes.FailedPrecondition, "legacy prompt evaluation Run execution is read-only")
}

func (s *AIExplanationAutomationService) executePromptEvaluationStepV2(
	ctx context.Context,
	request *interpretationpb.ExecutePromptEvaluationStepRequest,
	runID meta.ID,
	eventID string,
) (*interpretationpb.ExecutePromptEvaluationStepResponse, error) {
	command := aievaluation.OnlineStepV2Command{
		RunID: runID, ExecutionKind: domainevaluation.EvidenceExecutionKind(strings.TrimSpace(request.GetExecutionKind())),
		CaseID: strings.TrimSpace(request.GetCaseId()), SlotOrdinal: int(request.GetSlotOrdinal()),
		CandidateID: strings.TrimSpace(request.GetCandidateId()), ExecutionOrdinal: int(request.GetExecutionOrdinal()),
		Owner: eventID, RequestedOrgID: request.GetOrgId(), RequestedBy: strings.TrimSpace(request.GetRequestedBy()),
	}
	result, err := s.evaluationRunner.RunStepV2(ctx, command)
	if err != nil {
		if errors.Is(err, aievaluation.ErrAttemptExecutionBusy) {
			return nil, status.Error(codes.Aborted, "prompt evaluation v2 execution is leased")
		}
		slog.ErrorContext(ctx, "AI explanation prompt evaluation v2 step failed",
			slog.String("run_id", runID.String()), slog.String("execution_kind", request.GetExecutionKind()),
			slog.String("case_id", request.GetCaseId()), slog.Int("slot_ordinal", int(request.GetSlotOrdinal())),
			slog.String("candidate_id", request.GetCandidateId()), slog.Int("execution_ordinal", int(request.GetExecutionOrdinal())),
			slog.String("event_id", eventID), slog.String("error", err.Error()),
		)
		return nil, status.Error(codes.Internal, "AI explanation prompt evaluation v2 step failed")
	}
	return mapPromptEvaluationStepV2Result(request, command, result)
}

func validatePromptEvaluationStepRequest(request *interpretationpb.ExecutePromptEvaluationStepRequest) error {
	if request == nil || request.GetOrgId() <= 0 || strings.TrimSpace(request.GetRunId()) == "" ||
		strings.TrimSpace(request.GetCaseId()) == "" || strings.TrimSpace(request.GetRequestedBy()) == "" ||
		strings.TrimSpace(request.GetEventId()) == "" {
		return fmt.Errorf("prompt evaluation step address and audit are required")
	}
	version := strings.TrimSpace(request.GetEvidenceVersion())
	if version == eventpayload.AIExplanationPromptEvaluationEvidenceVersionV2 {
		kind := domainevaluation.EvidenceExecutionKind(strings.TrimSpace(request.GetExecutionKind()))
		if request.GetAttempt() != 0 || strings.TrimSpace(request.GetRecheckId()) != "" || !kind.IsValid() ||
			request.GetSlotOrdinal() < 1 || request.GetSlotOrdinal() > int32(domainevaluation.RequiredRepetitionsPerCase) ||
			request.GetExecutionOrdinal() < 1 || request.GetExecutionOrdinal() > 2 ||
			(kind == domainevaluation.EvidenceExecutionGeneration && strings.TrimSpace(request.GetCandidateId()) != "") ||
			(kind == domainevaluation.EvidenceExecutionSemantic && strings.TrimSpace(request.GetCandidateId()) == "") {
			return fmt.Errorf("prompt evaluation v2 exact execution address is required")
		}
		return nil
	}
	if version != "" || request.GetAttempt() < 1 || strings.TrimSpace(request.GetExecutionKind()) != "" ||
		request.GetSlotOrdinal() != 0 || strings.TrimSpace(request.GetCandidateId()) != "" || request.GetExecutionOrdinal() != 0 {
		return fmt.Errorf("prompt evaluation evidence version is invalid")
	}
	return nil
}

func mapPromptEvaluationRecheckResult(request *interpretationpb.ExecutePromptEvaluationStepRequest, result *aievaluation.OnlineRecheckResult) (*interpretationpb.ExecutePromptEvaluationStepResponse, error) {
	if result == nil || result.Recheck == nil || result.Recheck.ID().String() != request.GetRecheckId() ||
		result.Recheck.SourceRunID().String() != request.GetRunId() || result.Recheck.SourceCaseID() != request.GetCaseId() ||
		result.Recheck.SourceAttempt() != int(request.GetAttempt()) {
		return nil, status.Error(codes.Internal, "prompt evaluation recheck returned mismatched evidence")
	}
	switch result.Status {
	case aievaluation.OnlineRecheckCompleted, aievaluation.OnlineRecheckFailed,
		aievaluation.OnlineRecheckResultUnknown, aievaluation.OnlineRecheckAlreadyCompleted:
	default:
		return nil, status.Error(codes.Internal, "prompt evaluation recheck returned unsupported status")
	}
	if !result.Recheck.Status().IsTerminal() {
		return nil, status.Error(codes.Internal, "prompt evaluation recheck did not persist terminal evidence")
	}
	return &interpretationpb.ExecutePromptEvaluationStepResponse{
		Success: true, RunId: request.GetRunId(), CaseId: request.GetCaseId(), Attempt: request.GetAttempt(),
		Status: string(result.Status), RunStatus: string(result.Recheck.Status()), RecheckId: request.GetRecheckId(),
	}, nil
}

func mapPromptEvaluationStepV2Result(
	request *interpretationpb.ExecutePromptEvaluationStepRequest,
	command aievaluation.OnlineStepV2Command,
	result *aievaluation.OnlineStepV2Result,
) (*interpretationpb.ExecutePromptEvaluationStepResponse, error) {
	if result == nil || result.Evidence == nil || result.Evidence.RunID.String() != request.GetRunId() {
		return nil, status.Error(codes.Internal, "prompt evaluation v2 step returned mismatched evidence")
	}
	switch result.Status {
	case aievaluation.OnlineStepV2Progressed, aievaluation.OnlineStepV2AlreadyCompleted,
		aievaluation.OnlineStepV2AwaitingReview, aievaluation.OnlineStepV2Blocked, aievaluation.OnlineStepV2Canceled:
	default:
		return nil, status.Error(codes.Internal, "prompt evaluation v2 step returned unsupported status")
	}
	if result.Status != aievaluation.OnlineStepV2Canceled && !result.Evidence.HasTerminalExecution(command.Action()) {
		return nil, status.Error(codes.Internal, "prompt evaluation v2 step did not persist its target")
	}
	if result.Status == aievaluation.OnlineStepV2AwaitingReview && result.Evidence.Status != domainevaluation.EvidenceStatusAwaitingReview ||
		result.Status == aievaluation.OnlineStepV2Blocked && result.Evidence.Status != domainevaluation.EvidenceStatusBlocked ||
		result.Status == aievaluation.OnlineStepV2Canceled && result.Evidence.Status != domainevaluation.EvidenceStatusCanceled {
		return nil, status.Error(codes.Internal, "prompt evaluation v2 step returned inconsistent status")
	}
	return &interpretationpb.ExecutePromptEvaluationStepResponse{
		Success: true, RunId: request.GetRunId(), CaseId: request.GetCaseId(), Status: string(result.Status),
		RunStatus: string(result.Evidence.Status), EvidenceVersion: request.GetEvidenceVersion(),
		ExecutionKind: request.GetExecutionKind(), SlotOrdinal: request.GetSlotOrdinal(),
		CandidateId: request.GetCandidateId(), ExecutionOrdinal: request.GetExecutionOrdinal(),
	}, nil
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
