package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

const evaluationRunRetryHint = "check runtime_checkpoint table (scope=evaluation_run) for latest attempt and retryable flag"

// handleEvaluationRequested starts Evaluation from an explicit Evaluation event.
func handleEvaluationRequested(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		var data eventpayload.EvaluationRequestedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse evaluation requested event: %w", err)
		}

		deps.Logger.Info("received evaluation requested event",
			"event_id", env.ID,
			"org_id", data.OrgID,
			"assessment_id", data.AssessmentID,
			"testee_id", data.TesteeID,
			"questionnaire_code", data.QuestionnaireCode,
			"answersheet_id", data.AnswerSheetID,
			"model_kind", data.ModelKind,
			"model_algorithm", data.ModelAlgorithm,
			"payload_gate", string(data.ClassifyPayloadGate()),
			"has_model_identity", data.HasModelIdentity(),
		)
		// EV-R015: do not ACK on missing payload model identity. Canonical
		// Assessment.NeedsEvaluation decides skip vs evaluate inside Execute.
		gate := data.ClassifyPayloadGate()
		resilience.ObserveEvaluationPayloadGate(string(gate))
		switch gate {
		case eventpayload.PayloadGateInvalid:
			return fmt.Errorf("invalid evaluation.requested: assessment_id must be positive")
		case eventpayload.PayloadGateLegacyIncomplete:
			deps.Logger.Warn("evaluation.requested missing model identity; forwarding to Execute for Assessment authority",
				"event_id", env.ID,
				"assessment_id", data.AssessmentID,
				"payload_gate", string(gate),
			)
		}
		if deps.DisableAutomaticRetry && data.AttemptOrigin == "automatic" {
			deps.Logger.Warn("automatic evaluation retry disabled by emergency switch", "event_id", env.ID, "assessment_id", data.AssessmentID)
			return ErrAutomaticRetryPaused
		}
		if deps.EvaluationWorkerClient == nil {
			return fmt.Errorf("evaluation worker client is not available: cannot evaluate request for assessment %d", data.AssessmentID)
		}

		assessmentID, err := safeconv.Int64ToUint64(data.AssessmentID)
		if err != nil {
			return fmt.Errorf("invalid assessment id in evaluation requested event: %w", err)
		}
		if deps.ReportStatusReporter != nil {
			answerSheetID, convErr := strconv.ParseUint(data.AnswerSheetID, 10, 64)
			if convErr == nil {
				deps.ReportStatusReporter.SetProcessing(ctx, reportstatus.AssessmentKey(assessmentID), reportstatus.AssessmentKey(answerSheetID), "processing")
			}
		}

		callCtx := outgoingRetryAuthorization(ctx, env.ID, data.ExpectedAttempt, data.AttemptOrigin, data.ActionRequestID, data.Mode)
		resp, err := deps.EvaluationWorkerClient.ExecuteEvaluation(callCtx, assessmentID)
		if err != nil {
			return fmt.Errorf("failed to evaluate assessment: %w", err)
		}
		if err := handleEvaluateAssessmentResponse(resp); err != nil {
			deps.Logger.Warn("evaluation returned retryable failure", slog.Int64("assessment_id", data.AssessmentID), slog.String("error", err.Error()), slog.String("evaluation_run_hint", evaluationRunRetryHint))
			return err
		}
		deps.Logger.Info("evaluation request handled",
			slog.String("event_id", env.ID),
			slog.Int64("assessment_id", data.AssessmentID),
			slog.String("answersheet_id", data.AnswerSheetID),
			slog.String("status", resp.GetStatus()),
			slog.String("evaluation_run_id", resp.GetRunId()),
			slog.String("outcome_id", resp.GetOutcomeId()),
		)
		return nil
	}
}

// handleEvaluationFailed projects a failed Evaluation without treating it as a
// report failure.
func handleEvaluationFailed(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		var data eventpayload.EvaluationFailedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse evaluation failed event: %w", err)
		}
		deps.Logger.Error("evaluation failed",
			slog.String("event_id", env.ID),
			slog.Int64("org_id", data.OrgID),
			slog.Int64("assessment_id", data.AssessmentID),
			slog.Uint64("testee_id", data.TesteeID),
			slog.String("reason", data.Reason),
			slog.Time("failed_at", data.FailedAt),
		)
		if deps.InternalClient == nil {
			return nil
		}
		assessmentID, err := safeconv.Int64ToUint64(data.AssessmentID)
		if err != nil {
			return fmt.Errorf("invalid assessment id in evaluation failed event: %w", err)
		}
		if deps.ReportStatusReporter != nil {
			deps.ReportStatusReporter.SetFailed(ctx, reportstatus.AssessmentKey(assessmentID), "", "evaluation_failed", data.Reason)
		}
		return nil
	}
}
