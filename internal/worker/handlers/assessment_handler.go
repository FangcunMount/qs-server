package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"google.golang.org/protobuf/types/known/timestamppb"
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

		deps.Logger.Debug("evaluation requested detail",
			"event_id", env.ID,
			"org_id", data.OrgID,
			"assessment_id", data.AssessmentID,
			"testee_id", data.TesteeID,
			"questionnaire_code", data.QuestionnaireCode,
			"answersheet_id", data.AnswerSheetID,
			"model_kind", data.ModelKind,
			"model_sub_kind", data.ModelSubKind,
			"model_algorithm", data.ModelAlgorithm,
			"needs_evaluation", data.NeedsEvaluation(),
		)
		if !data.NeedsEvaluation() {
			return nil
		}
		if deps.InternalClient == nil {
			return fmt.Errorf("internal client is not available: cannot evaluate request for assessment %d", data.AssessmentID)
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

		resp, err := deps.InternalClient.EvaluateAssessment(ctx, assessmentID)
		if err != nil {
			return fmt.Errorf("failed to evaluate assessment: %w", err)
		}
		if err := handleEvaluateAssessmentResponse(resp); err != nil {
			deps.Logger.Warn("evaluation returned retryable failure", slog.Int64("assessment_id", data.AssessmentID), slog.String("error", err.Error()), slog.String("evaluation_run_hint", evaluationRunRetryHint))
			return err
		}
		return nil
	}
}

// handleEvaluationFailed projects a failed Evaluation without treating it as a
// report failure.
func handleEvaluationFailed(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
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
		if _, err := deps.InternalClient.ProjectBehaviorEvent(ctx, &pb.ProjectBehaviorEventRequest{
			EventId:       env.ID,
			EventType:     eventType,
			OrgId:         data.OrgID,
			TesteeId:      data.TesteeID,
			AssessmentId:  assessmentID,
			FailureReason: data.Reason,
			OccurredAt:    timestamppb.New(data.FailedAt),
		}); err != nil {
			return fmt.Errorf("failed to project evaluation failed behavior: %w", err)
		}
		return nil
	}
}
