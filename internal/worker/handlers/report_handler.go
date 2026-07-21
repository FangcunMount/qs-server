package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"google.golang.org/grpc/metadata"
)

func handleInterpretationReportGenerated(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		return handleReportGeneratedOutcome(ctx, deps, payload)
	}
}

func handleInterpretationReportFailed(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		var data eventoutcome.ReportFailedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse interpretation report failed event: %w", err)
		}
		disposition := failedDisposition(data)
		deps.Logger.Warn("interpretation report failed",
			slog.String("event_id", env.ID),
			slog.String("generation_id", data.GenerationID),
			slog.String("run_id", data.RunID),
			slog.String("assessment_id", data.AssessmentID),
			slog.String("outcome_id", data.OutcomeID),
			slog.Uint64("testee_id", data.TesteeID),
			slog.Uint64("attempt", uint64(data.Attempt)),
			slog.String("failure_kind", data.FailureKind),
			slog.String("failure_code", data.FailureCode),
			slog.Bool("retryable", data.Retryable),
			slog.String("disposition", disposition),
			slog.String("safe_reason", data.SafeReason),
			slog.Time("failed_at", data.FailedAt),
		)
		switch disposition {
		case string(retrygovernance.DispositionAutomatic):
			// Automatic recovery continues; keep patient projection in-flight.
			return nil
		case string(retrygovernance.DispositionManualRequired):
			return markReportTemporarilyUnavailable(ctx, deps, data.AssessmentID, "waiting_manual_action", data.SafeReason)
		default:
			// Terminal or legacy events without retry_decision that are non-retryable.
			if disposition == string(retrygovernance.DispositionTerminal) || !data.Retryable {
				return markReportFailed(ctx, deps, data.AssessmentID, "interpretation_report_failed", data.SafeReason)
			}
			return nil
		}
	}
}

func failedDisposition(data eventoutcome.ReportFailedPayload) string {
	if data.RetryDecision != nil && data.RetryDecision.Disposition != "" {
		return data.RetryDecision.Disposition
	}
	if data.Retryable {
		return string(retrygovernance.DispositionAutomatic)
	}
	return string(retrygovernance.DispositionTerminal)
}

func handleInterpretationRetryRequested(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		var data eventoutcome.InterpretationRetryRequestedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse interpretation retry requested event: %w", err)
		}
		if deps.DisableAutomaticRetry && data.AttemptOrigin == "automatic" {
			deps.Logger.Warn("automatic interpretation retry disabled by emergency switch", "event_id", env.ID, "generation_id", data.GenerationID)
			return ErrAutomaticRetryPaused
		}
		if deps.InterpretationAutomationClient == nil {
			return fmt.Errorf("interpretation automation client is not available")
		}
		callCtx := metadata.AppendToOutgoingContext(ctx, "x-event-id", env.ID)
		callCtx = outgoingRetryAuthorization(callCtx, env.ID, data.ExpectedAttempt, data.AttemptOrigin, data.ActionRequestID, data.Mode)
		resp, err := deps.InterpretationAutomationClient.GenerateReportFromOutcome(callCtx, data.OutcomeID)
		if err != nil {
			return fmt.Errorf("retry interpretation generation: %w", err)
		}
		return handleGenerateReportResponse(resp)
	}
}

func handleReportGeneratedOutcome(ctx context.Context, deps *Dependencies, payload []byte) error {
	var data eventoutcome.ReportGeneratedPayload
	env, err := ParseEventData(payload, &data)
	if err != nil {
		return fmt.Errorf("failed to parse report generated outcome event: %w", err)
	}
	deps.Logger.Info("processing report generated outcome",
		slog.String("event_id", env.ID),
		slog.String("report_id", data.ReportID),
		slog.String("generation_id", data.GenerationID),
		slog.String("run_id", data.RunID),
		slog.String("template_version", data.TemplateVersion),
		slog.String("builder_identity", data.BuilderIdentity),
		slog.String("assessment_id", data.AssessmentID),
		slog.Uint64("testee_id", data.TesteeID),
		slog.String("level_code", levelCode(data.Level)),
		slog.String("severity", levelSeverity(data.Level)),
	)
	if err := markReportCompleted(ctx, deps, data.AssessmentID, data.ReportID); err != nil {
		return err
	}
	riskLevel := attentionRiskLevelFromOutcome(data.Level)
	handleHighRiskAlert(deps, riskLevel, primaryScoreValue(data.PrimaryScore), data.ReportID, data.TesteeID)
	if deps.InternalClient != nil {
		syncAssessmentAttention(ctx, deps, data.TesteeID, riskLevel, isHighRiskOutcomeLevel(data.Level))
	}
	return nil
}

func markReportCompleted(ctx context.Context, deps *Dependencies, assessmentID, reportID string) error {
	if deps.ReportStatusReporter == nil {
		return nil
	}
	id, err := strconv.ParseUint(assessmentID, 10, 64)
	if err != nil || id == 0 {
		return fmt.Errorf("invalid assessment id in report generated event: %q", assessmentID)
	}
	deps.ReportStatusReporter.SetCompleted(ctx, reportstatus.AssessmentKey(id), "", reportID)
	return nil
}

func markReportFailed(ctx context.Context, deps *Dependencies, assessmentID, reason, message string) error {
	if deps.ReportStatusReporter == nil {
		return nil
	}
	id, err := strconv.ParseUint(assessmentID, 10, 64)
	if err != nil || id == 0 {
		return fmt.Errorf("invalid assessment id in report failed event: %q", assessmentID)
	}
	deps.ReportStatusReporter.SetFailed(ctx, reportstatus.AssessmentKey(id), "", reason, message)
	return nil
}

func markReportTemporarilyUnavailable(ctx context.Context, deps *Dependencies, assessmentID, reason, message string) error {
	if deps.ReportStatusReporter == nil {
		return nil
	}
	id, err := strconv.ParseUint(assessmentID, 10, 64)
	if err != nil || id == 0 {
		return fmt.Errorf("invalid assessment id in report failed event: %q", assessmentID)
	}
	if message == "" {
		message = "报告暂不可用，请稍后重试"
	}
	deps.ReportStatusReporter.SetTemporarilyUnavailable(ctx, reportstatus.AssessmentKey(id), "", reason, message)
	return nil
}

func handleHighRiskAlert(deps *Dependencies, riskLevel string, totalScore float64, reportID string, testeeID uint64) {
	if !isHighRiskRiskLevel(riskLevel) {
		return
	}
	deps.Logger.Warn("HIGH RISK REPORT GENERATED",
		slog.String("report_id", reportID),
		slog.Uint64("testee_id", testeeID),
		slog.String("risk_level", riskLevel),
		slog.Float64("total_score", totalScore),
	)
}

func syncAssessmentAttention(ctx context.Context, deps *Dependencies, testeeID uint64, riskLevel string, markKeyFocus bool) {
	resp, err := deps.InternalClient.SyncAssessmentAttention(
		ctx,
		&pb.SyncAssessmentAttentionRequest{
			TesteeId:     testeeID,
			RiskLevel:    riskLevel,
			MarkKeyFocus: markKeyFocus,
		},
	)
	if err != nil {
		deps.Logger.Warn("failed to sync assessment attention",
			slog.Uint64("testee_id", testeeID),
			slog.String("error", err.Error()),
		)
		return
	}
	deps.Logger.Info("assessment attention synced successfully",
		slog.Uint64("testee_id", testeeID),
		slog.Bool("key_focus_marked", resp.KeyFocusMarked),
	)
}

func primaryScoreValue(score *eventoutcome.ScoreValue) float64 {
	if score == nil {
		return 0
	}
	return score.Value
}

func levelCode(level *eventoutcome.ResultLevel) string {
	if level == nil {
		return ""
	}
	return level.Code
}

func levelSeverity(level *eventoutcome.ResultLevel) string {
	if level == nil {
		return ""
	}
	return level.Severity
}
