package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"google.golang.org/grpc/metadata"
)

func handleAIExplanationRequested(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		request, envelopeID, err := decodeAIExplanationExecutionRequest(eventType, payload)
		if err != nil {
			return err
		}
		if deps.AIExplanationAutomationClient == nil {
			return fmt.Errorf("AI explanation automation client is not available")
		}
		callCtx := metadata.AppendToOutgoingContext(ctx, "x-event-id", envelopeID)
		response, err := deps.AIExplanationAutomationClient.ExecuteAIExplanation(callCtx, request)
		if err != nil {
			return fmt.Errorf("execute AI explanation generation %s: %w", request.GetGenerationId(), err)
		}
		if response == nil || response.GetGenerationId() != request.GetGenerationId() {
			return fmt.Errorf("AI explanation automation returned an invalid generation response")
		}
		switch response.GetStatus() {
		case "generated", "processing":
			if !response.GetSuccess() {
				return fmt.Errorf("AI explanation automation returned unsuccessful %s response", response.GetStatus())
			}
			deps.Logger.Info("AI explanation request handled",
				slog.String("event_id", envelopeID), slog.String("generation_id", request.GetGenerationId()),
				slog.String("run_id", response.GetRunId()), slog.String("artifact_id", response.GetArtifactId()),
				slog.String("status", response.GetStatus()),
			)
			return nil
		case "failed":
			if response.GetSuccess() || strings.TrimSpace(response.GetFailureCode()) == "" || strings.TrimSpace(response.GetSafeMessage()) == "" {
				return fmt.Errorf("AI explanation automation returned an invalid failed response")
			}
			// The failed Generation/Run and failed event were committed before this
			// response, so ACK the requested event instead of starting a second call.
			deps.Logger.Warn("AI explanation reached persisted failure",
				slog.String("event_id", envelopeID), slog.String("generation_id", request.GetGenerationId()),
				slog.String("run_id", response.GetRunId()), slog.String("failure_kind", response.GetFailureKind()),
				slog.String("failure_code", response.GetFailureCode()), slog.Bool("retryable", response.GetRetryable()),
				slog.String("safe_message", response.GetSafeMessage()),
			)
			return nil
		default:
			return fmt.Errorf("AI explanation automation returned unsupported status %q", response.GetStatus())
		}
	}
}

func decodeAIExplanationExecutionRequest(eventType string, payload []byte) (*interpretationpb.ExecuteAIExplanationRequest, string, error) {
	switch eventType {
	case eventcatalog.AIExplanationRequested:
		var data eventpayload.AIExplanationRequestedData
		envelope, err := ParseEventData(payload, &data)
		if err != nil {
			return nil, "", fmt.Errorf("parse AI explanation requested event: %w", err)
		}
		if err := validateAIExplanationRequested(data); err != nil {
			return nil, "", err
		}
		return &interpretationpb.ExecuteAIExplanationRequest{
			GenerationId: data.GenerationID, TraceId: envelope.ID, EventId: envelope.ID,
		}, envelope.ID, nil
	case eventcatalog.AIExplanationRetryRequested:
		var data eventpayload.AIExplanationRetryRequestedData
		envelope, err := ParseEventData(payload, &data)
		if err != nil {
			return nil, "", fmt.Errorf("parse AI explanation retry requested event: %w", err)
		}
		if err := validateAIExplanationRetryRequested(data); err != nil {
			return nil, "", err
		}
		return &interpretationpb.ExecuteAIExplanationRequest{
			GenerationId: data.GenerationID, TraceId: envelope.ID, EventId: envelope.ID,
			ExpectedAttempt: int32(data.ExpectedAttempt), AttemptOrigin: data.AttemptOrigin,
			ActionRequestId: data.ActionRequestID,
		}, envelope.ID, nil
	case eventcatalog.AIExplanationLeaseRecoveryRequested:
		var data eventpayload.AIExplanationLeaseRecoveryRequestedData
		envelope, err := ParseEventData(payload, &data)
		if err != nil {
			return nil, "", fmt.Errorf("parse AI explanation lease recovery requested event: %w", err)
		}
		if err := validateAIExplanationLeaseRecoveryRequested(data); err != nil {
			return nil, "", err
		}
		return &interpretationpb.ExecuteAIExplanationRequest{
			GenerationId: data.GenerationID, TraceId: envelope.ID, EventId: envelope.ID,
			ExpectedRunId: data.RunID, ExpectedLeaseExpiresAt: data.ExpectedLeaseExpiresAt.UTC().Format(time.RFC3339Nano),
			ExpectedInvocationPhase: data.InvocationPhase,
		}, envelope.ID, nil
	default:
		return nil, "", fmt.Errorf("unsupported AI explanation execution event %q", eventType)
	}
}

func handleAIExplanationPromptEvaluationStep(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		var data eventpayload.AIExplanationPromptEvaluationStepRequestedData
		envelope, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("parse AI explanation prompt evaluation step event: %w", err)
		}
		if err := validateAIExplanationPromptEvaluationStep(data); err != nil {
			return err
		}
		if deps.AIExplanationAutomationClient == nil {
			return fmt.Errorf("AI explanation automation client is not available")
		}
		callCtx := metadata.AppendToOutgoingContext(ctx, "x-event-id", envelope.ID)
		response, err := deps.AIExplanationAutomationClient.ExecutePromptEvaluationStep(callCtx, &interpretationpb.ExecutePromptEvaluationStepRequest{
			OrgId: data.OrgID, RunId: data.RunID, CaseId: data.CaseID, Attempt: int32(data.Attempt),
			RequestedBy: data.RequestedBy, EventId: envelope.ID, RecheckId: data.RecheckID,
			EvidenceVersion: data.EvidenceVersion, ExecutionKind: data.ExecutionKind, SlotOrdinal: int32(data.SlotOrdinal),
			CandidateId: data.CandidateID, ExecutionOrdinal: int32(data.ExecutionOrdinal),
		})
		if err != nil {
			return fmt.Errorf("execute AI explanation prompt evaluation run %s case %s attempt %d: %w", data.RunID, data.CaseID, data.Attempt, err)
		}
		if response == nil || !response.GetSuccess() || response.GetRunId() != data.RunID ||
			response.GetCaseId() != data.CaseID || response.GetAttempt() != int32(data.Attempt) || response.GetRecheckId() != data.RecheckID ||
			response.GetEvidenceVersion() != data.EvidenceVersion || response.GetExecutionKind() != data.ExecutionKind ||
			response.GetSlotOrdinal() != int32(data.SlotOrdinal) || response.GetCandidateId() != data.CandidateID ||
			response.GetExecutionOrdinal() != int32(data.ExecutionOrdinal) {
			return fmt.Errorf("AI explanation prompt evaluation automation returned an invalid response")
		}
		switch response.GetStatus() {
		case "progressed", "already_completed", "awaiting_review", "blocked", "canceled",
			"recheck_completed", "recheck_failed", "recheck_result_unknown", "recheck_already_completed":
			deps.Logger.Info("AI explanation prompt evaluation step handled",
				slog.String("event_id", envelope.ID), slog.String("run_id", data.RunID),
				slog.String("case_id", data.CaseID), slog.Int("attempt", data.Attempt),
				slog.String("evidence_version", data.EvidenceVersion), slog.String("execution_kind", data.ExecutionKind),
				slog.Int("slot_ordinal", data.SlotOrdinal), slog.String("candidate_id", data.CandidateID),
				slog.Int("execution_ordinal", data.ExecutionOrdinal),
				slog.String("recheck_id", data.RecheckID),
				slog.String("status", response.GetStatus()), slog.String("run_status", response.GetRunStatus()),
				slog.String("next_case_id", response.GetNextCaseId()), slog.Int("next_attempt", int(response.GetNextAttempt())),
			)
			return nil
		default:
			return fmt.Errorf("AI explanation prompt evaluation automation returned unsupported status %q", response.GetStatus())
		}
	}
}

// handleAIExplanationTerminal validates and audits terminal references. It is
// deliberately not a projection handler: clients read canonical Generation and
// Artifact state, and an ACK here proves only this audit consumer completed.
func handleAIExplanationTerminal(deps *Dependencies) HandlerFunc {
	return func(_ context.Context, eventType string, payload []byte) error {
		switch eventType {
		case eventcatalog.AIExplanationGenerated:
			var data eventpayload.AIExplanationGeneratedData
			envelope, err := ParseEventData(payload, &data)
			if err != nil {
				return fmt.Errorf("parse AI explanation generated event: %w", err)
			}
			if err := validateAIExplanationGenerated(data); err != nil {
				return err
			}
			deps.Logger.Info("AI explanation generated terminal fact",
				slog.String("event_id", envelope.ID), slog.String("generation_id", data.GenerationID),
				slog.String("run_id", data.RunID), slog.String("artifact_id", data.ArtifactID),
				slog.String("assessment_id", data.AssessmentID), slog.Uint64("testee_id", data.TesteeID),
			)
			return nil
		case eventcatalog.AIExplanationFailed:
			var data eventpayload.AIExplanationFailedData
			envelope, err := ParseEventData(payload, &data)
			if err != nil {
				return fmt.Errorf("parse AI explanation failed event: %w", err)
			}
			if err := validateAIExplanationFailed(data); err != nil {
				return err
			}
			deps.Logger.Warn("AI explanation failed terminal fact",
				slog.String("event_id", envelope.ID), slog.String("generation_id", data.GenerationID),
				slog.String("run_id", data.RunID), slog.Int("attempt", data.Attempt),
				slog.String("failure_kind", data.FailureKind), slog.String("failure_code", data.FailureCode),
				slog.Bool("retryable", data.Retryable), slog.String("safe_reason", data.SafeReason),
			)
			return nil
		default:
			return fmt.Errorf("unsupported AI explanation terminal event %q", eventType)
		}
	}
}

func validateAIExplanationRequested(data eventpayload.AIExplanationRequestedData) error {
	if err := validateAIExplanationCommon(data.OrgID, data.GenerationID, data.AssessmentID, data.TesteeID, data.SourceReportID, data.Audience); err != nil {
		return fmt.Errorf("invalid AI explanation requested event: %w", err)
	}
	if data.RequestedAt.IsZero() {
		return fmt.Errorf("invalid AI explanation requested event: requested_at is required")
	}
	return nil
}

func validateAIExplanationRetryRequested(data eventpayload.AIExplanationRetryRequestedData) error {
	if err := validateAIExplanationCommon(data.OrgID, data.GenerationID, data.AssessmentID, data.TesteeID, data.SourceReportID, data.Audience); err != nil {
		return fmt.Errorf("invalid AI explanation retry requested event: %w", err)
	}
	if err := requirePositiveAIExplanationIDs(data.FailedRunID); err != nil || data.ExpectedAttempt < 1 ||
		data.NextAttempt != data.ExpectedAttempt+1 || data.AttemptOrigin != "manual" ||
		strings.TrimSpace(data.ActionRequestID) == "" || data.RequestedAt.IsZero() {
		return fmt.Errorf("invalid AI explanation retry requested event: authorization is required")
	}
	return nil
}

func validateAIExplanationLeaseRecoveryRequested(data eventpayload.AIExplanationLeaseRecoveryRequestedData) error {
	if data.OrgID == 0 || data.Attempt < 1 || data.ExpectedLeaseExpiresAt.IsZero() || data.RequestedAt.IsZero() ||
		data.RequestedAt.Before(data.ExpectedLeaseExpiresAt) ||
		(data.InvocationPhase != "prepared" && data.InvocationPhase != "dispatching") {
		return fmt.Errorf("invalid AI explanation lease recovery requested event: recovery proof is required")
	}
	if err := requirePositiveAIExplanationIDs(data.GenerationID, data.RunID); err != nil {
		return fmt.Errorf("invalid AI explanation lease recovery requested event: %w", err)
	}
	return nil
}

func validateAIExplanationPromptEvaluationStep(data eventpayload.AIExplanationPromptEvaluationStepRequestedData) error {
	runID, err := meta.ParseID(data.RunID)
	if data.OrgID <= 0 || err != nil || runID.IsZero() || strings.TrimSpace(data.CaseID) == "" ||
		strings.TrimSpace(data.RequestedBy) == "" || data.RequestedAt.IsZero() {
		return fmt.Errorf("invalid AI explanation prompt evaluation step event: address and audit are required")
	}
	version := strings.TrimSpace(data.EvidenceVersion)
	if version == eventpayload.AIExplanationPromptEvaluationEvidenceVersionV2 {
		kind := strings.TrimSpace(data.ExecutionKind)
		validKind := kind == eventpayload.AIExplanationPromptEvaluationExecutionKindGeneration ||
			kind == eventpayload.AIExplanationPromptEvaluationExecutionKindSemantic
		if data.Attempt != 0 || strings.TrimSpace(data.RecheckID) != "" || !validKind ||
			data.SlotOrdinal < 1 || data.SlotOrdinal > eventpayload.AIExplanationPromptEvaluationRequiredSlotsPerCase ||
			data.ExecutionOrdinal < 1 || data.ExecutionOrdinal > eventpayload.AIExplanationPromptEvaluationMaxExecutionsPerTarget ||
			(kind == eventpayload.AIExplanationPromptEvaluationExecutionKindGeneration && strings.TrimSpace(data.CandidateID) != "") ||
			(kind == eventpayload.AIExplanationPromptEvaluationExecutionKindSemantic && strings.TrimSpace(data.CandidateID) == "") {
			return fmt.Errorf("invalid AI explanation prompt evaluation v2 step event: exact execution address is required")
		}
		return nil
	}
	if version != "" || data.Attempt < 1 || strings.TrimSpace(data.ExecutionKind) != "" || data.SlotOrdinal != 0 ||
		strings.TrimSpace(data.CandidateID) != "" || data.ExecutionOrdinal != 0 {
		return fmt.Errorf("invalid AI explanation prompt evaluation step event: evidence version is invalid")
	}
	if strings.TrimSpace(data.RecheckID) != "" {
		recheckID, parseErr := meta.ParseID(data.RecheckID)
		if parseErr != nil || recheckID.IsZero() {
			return fmt.Errorf("invalid AI explanation prompt evaluation step event: recheck identity is invalid")
		}
	}
	return nil
}

func validateAIExplanationGenerated(data eventpayload.AIExplanationGeneratedData) error {
	if err := validateAIExplanationCommon(data.OrgID, data.GenerationID, data.AssessmentID, data.TesteeID, data.SourceReportID, data.Audience); err != nil {
		return fmt.Errorf("invalid AI explanation generated event: %w", err)
	}
	if err := requirePositiveAIExplanationIDs(data.RunID, data.ArtifactID); err != nil || data.GeneratedAt.IsZero() {
		return fmt.Errorf("invalid AI explanation generated event: terminal references are required")
	}
	return nil
}

func validateAIExplanationFailed(data eventpayload.AIExplanationFailedData) error {
	if err := validateAIExplanationCommon(data.OrgID, data.GenerationID, data.AssessmentID, data.TesteeID, data.SourceReportID, data.Audience); err != nil {
		return fmt.Errorf("invalid AI explanation failed event: %w", err)
	}
	if err := requirePositiveAIExplanationIDs(data.RunID); err != nil || data.Attempt < 1 || strings.TrimSpace(data.FailureKind) == "" || strings.TrimSpace(data.FailureCode) == "" || strings.TrimSpace(data.SafeReason) == "" || data.FailedAt.IsZero() {
		return fmt.Errorf("invalid AI explanation failed event: failure references are required")
	}
	return nil
}

func validateAIExplanationCommon(orgID int64, generationID, assessmentID string, testeeID uint64, sourceReportID, audience string) error {
	if orgID == 0 || testeeID == 0 {
		return fmt.Errorf("organization and testee are required")
	}
	if err := requirePositiveAIExplanationIDs(generationID, assessmentID, sourceReportID); err != nil {
		return err
	}
	if audience != "participant" && audience != "clinician" {
		return fmt.Errorf("audience is invalid")
	}
	return nil
}

func requirePositiveAIExplanationIDs(values ...string) error {
	for _, value := range values {
		id, err := meta.ParseID(value)
		if err != nil || id.IsZero() {
			return fmt.Errorf("positive identifier is required")
		}
	}
	return nil
}
