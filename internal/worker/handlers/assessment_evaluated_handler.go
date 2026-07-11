package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
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

		if deps.InternalClient == nil {
			return fmt.Errorf("internal client is not available: cannot generate report for assessment %d", data.AssessmentID)
		}

		if _, err := safeconv.Int64ToUint64(data.AssessmentID); err != nil {
			return fmt.Errorf("invalid assessment id in evaluation outcome committed event: %w", err)
		}
		if data.OutcomeID == "" {
			return fmt.Errorf("outcome id is required in evaluation outcome committed event for assessment %d", data.AssessmentID)
		}

		resp, err := deps.InternalClient.GenerateReportFromOutcome(ctx, data.OutcomeID)
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
					slog.String("status", resp.Status),
					slog.String("message", resp.Message),
				)
			} else {
				deps.Logger.Error("report generation returned unsuccessful response",
					slog.Int64("assessment_id", data.AssessmentID),
					slog.String("status", resp.Status),
					slog.String("message", resp.Message),
				)
			}
		}
		return handleGenerateReportResponse(resp)
	}
}
