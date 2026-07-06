package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

func handleAssessmentEvaluated(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		var data eventpayload.AssessmentEvaluatedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse assessment evaluated event: %w", err)
		}

		deps.Logger.Debug("assessment evaluated detail",
			"event_id", env.ID,
			"org_id", data.OrgID,
			"assessment_id", data.AssessmentID,
			"testee_id", data.TesteeID,
		)

		if deps.InternalClient == nil {
			return fmt.Errorf("internal client is not available: cannot generate report for assessment %d", data.AssessmentID)
		}

		assessmentID, err := safeconv.Int64ToUint64(data.AssessmentID)
		if err != nil {
			return fmt.Errorf("invalid assessment id in evaluated event: %w", err)
		}

		resp, err := deps.InternalClient.GenerateReportFromAssessment(ctx, assessmentID)
		if err != nil {
			deps.Logger.Error("failed to generate report from assessment",
				slog.Int64("assessment_id", data.AssessmentID),
				slog.String("error", err.Error()),
			)
			return fmt.Errorf("failed to generate report from assessment: %w", err)
		}
		if resp != nil && !resp.Success {
			if resp.Status == "failed" {
				deps.Logger.Warn("report generation failed and assessment marked failed",
					slog.Int64("assessment_id", data.AssessmentID),
					slog.String("status", resp.Status),
					slog.String("message", resp.Message),
				)
				return nil
			}
			deps.Logger.Error("report generation returned unsuccessful response",
				slog.Int64("assessment_id", data.AssessmentID),
				slog.String("status", resp.Status),
				slog.String("message", resp.Message),
			)
			return fmt.Errorf("report generation failed: %s", resp.Message)
		}
		return nil
	}
}
