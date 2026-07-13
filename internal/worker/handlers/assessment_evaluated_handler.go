package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc/metadata"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

func handleEvaluationOutcomeCommitted(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		var data eventpayload.EvaluationOutcomeCommittedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse evaluation outcome committed event: %w", err)
		}

		deps.Logger.Debug("evaluation outcome committed detail",
			"event_id", env.ID,
			"org_id", data.OrgID,
			"assessment_id", data.AssessmentID,
			"testee_id", data.TesteeID,
			"outcome_id", data.OutcomeID,
			"evaluation_run_id", data.EvaluationRunID,
		)

		if deps.InterpretationAutomationClient == nil {
			return fmt.Errorf("interpretation automation client is not available: cannot generate report for assessment %d", data.AssessmentID)
		}

		if _, err := safeconv.Int64ToUint64(data.AssessmentID); err != nil {
			return fmt.Errorf("invalid assessment id in evaluation outcome committed event: %w", err)
		}
		if data.OutcomeID == "" {
			return fmt.Errorf("outcome id is required in evaluation outcome committed event for assessment %d", data.AssessmentID)
		}

		callCtx := metadata.AppendToOutgoingContext(ctx, "x-event-id", env.ID)
		resp, err := deps.InterpretationAutomationClient.GenerateReportFromOutcome(callCtx, data.OutcomeID)
		if err != nil {
			deps.Logger.Error("failed to generate report from assessment",
				slog.Int64("assessment_id", data.AssessmentID),
				slog.String("error", err.Error()),
			)
			return fmt.Errorf("failed to generate report from assessment: %w", err)
		}
		if resp != nil && !resp.Success {
			if isTerminalReportGenerationStatus(resp.Status) {
				deps.Logger.Warn("report generation reached terminal status",
					slog.Int64("assessment_id", data.AssessmentID),
					slog.String("generation_id", resp.GetGenerationId()),
					slog.String("run_id", resp.GetRunId()),
					slog.String("status", resp.Status),
					slog.String("failure_kind", resp.GetFailureKind()),
					slog.String("failure_code", resp.GetFailureCode()),
					slog.Bool("retryable", resp.GetRetryable()),
					slog.String("message", resp.Message),
				)
			} else {
				deps.Logger.Error("report generation returned unsuccessful response",
					slog.Int64("assessment_id", data.AssessmentID),
					slog.String("generation_id", resp.GetGenerationId()),
					slog.String("run_id", resp.GetRunId()),
					slog.String("status", resp.Status),
					slog.String("failure_kind", resp.GetFailureKind()),
					slog.String("failure_code", resp.GetFailureCode()),
					slog.Bool("retryable", resp.GetRetryable()),
					slog.String("message", resp.Message),
				)
			}
		}
		return handleGenerateReportResponse(resp)
	}
}
