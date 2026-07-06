package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// handleAssessmentSubmitted 处理测评提交事件
// 业务逻辑：
// 1. 解析测评提交事件
// 2. 检查是否需要评估（有关联量表）
// 3. 调用 InternalClient 执行评估
func handleAssessmentSubmitted(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		var data eventpayload.AssessmentSubmittedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse assessment submitted event: %w", err)
		}

		deps.Logger.Debug("assessment submitted detail",
			"event_id", env.ID,
			"org_id", data.OrgID,
			"assessment_id", data.AssessmentID,
			"testee_id", data.TesteeID,
			"questionnaire_code", data.QuestionnaireCode,
			"answersheet_id", data.AnswerSheetID,
			"model_kind", data.ModelKind,
			"model_sub_kind", data.ModelSubKind,
			"model_algorithm", data.ModelAlgorithm,
			"scale_code", data.ScaleCode,
			"needs_evaluation", data.NeedsEvaluation(),
		)

		// 如果没有关联量表，无需评估
		if !data.NeedsEvaluation() {
			deps.Logger.Info("assessment does not need evaluation (no scale)",
				slog.Int64("assessment_id", data.AssessmentID),
			)
			return nil
		}

		if deps.InternalClient == nil {
			return fmt.Errorf("internal client is not available: cannot evaluate assessment %d", data.AssessmentID)
		}

		assessmentID, err := safeconv.Int64ToUint64(data.AssessmentID)
		if err != nil {
			return fmt.Errorf("invalid assessment id in submitted event: %w", err)
		}

		if deps.ReportStatusReporter != nil {
			answerSheetID, convErr := strconv.ParseUint(data.AnswerSheetID, 10, 64)
			if convErr == nil {
				deps.ReportStatusReporter.SetProcessing(
					ctx,
					reportstatus.AssessmentKey(assessmentID),
					reportstatus.AssessmentKey(answerSheetID),
					"processing",
				)
			}
		}

		// 调用 InternalClient 执行评估
		resp, err := deps.InternalClient.EvaluateAssessment(ctx, assessmentID)
		if err != nil {
			deps.Logger.Error("failed to evaluate assessment",
				slog.Int64("assessment_id", data.AssessmentID),
				slog.String("error", err.Error()),
			)
			return fmt.Errorf("failed to evaluate assessment: %w", err)
		}
		if resp != nil && !resp.Success {
			deps.Logger.Info("assessment evaluation skipped",
				slog.Int64("assessment_id", data.AssessmentID),
				slog.String("status", resp.Status),
				slog.String("message", resp.Message),
			)
			return nil
		}

		deps.Logger.Debug("assessment evaluation completed",
			slog.Int64("assessment_id", data.AssessmentID),
			"success", resp.Success,
			"status", resp.Status,
			"total_score", resp.TotalScore,
			"risk_level", resp.RiskLevel,
			"message", resp.Message,
		)

		return nil
	}
}

// handleAssessmentInterpreted 处理测评解读完成事件（outcome payload）。
func handleAssessmentInterpreted(deps *Dependencies) HandlerFunc {
	return func(_ context.Context, _ string, payload []byte) error {
		return handleAssessmentInterpretedOutcome(deps, payload)
	}
}

func handleAssessmentInterpretedOutcome(deps *Dependencies, payload []byte) error {
	var data eventoutcome.AssessmentInterpretedPayload
	_, err := ParseEventData(payload, &data)
	if err != nil {
		return fmt.Errorf("failed to parse assessment interpreted outcome event: %w", err)
	}
	deps.Logger.Debug("assessment interpreted outcome detail",
		"org_id", data.OrgID,
		"level_code", assessmentLevelCode(data.Level),
		"severity", assessmentLevelSeverity(data.Level),
		"is_high_risk", data.IsHighRisk(),
	)
	if data.IsHighRisk() {
		logAssessmentHighRisk(deps, data.AssessmentID, data.TesteeID, assessmentLevelCode(data.Level), assessmentPrimaryScoreValue(data.PrimaryScore))
	}
	return nil
}

func logAssessmentHighRisk(deps *Dependencies, assessmentID int64, testeeID uint64, riskLevel string, totalScore float64) {
	deps.Logger.Warn("HIGH RISK ALERT",
		"assessment_id", assessmentID,
		"testee_id", testeeID,
		"risk_level", riskLevel,
		"total_score", totalScore,
	)
}

func assessmentPrimaryScoreValue(score *eventoutcome.ScoreValue) float64 {
	if score == nil {
		return 0
	}
	return score.Value
}

func assessmentLevelCode(level *eventoutcome.ResultLevel) string {
	if level == nil {
		return ""
	}
	return level.Code
}

func assessmentLevelSeverity(level *eventoutcome.ResultLevel) string {
	if level == nil {
		return ""
	}
	return level.Severity
}

// handleAssessmentFailed 处理测评失败事件
func handleAssessmentFailed(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data eventpayload.AssessmentFailedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse assessment failed event: %w", err)
		}

		deps.Logger.Error("assessment failed",
			slog.String("event_id", env.ID),
			slog.Int64("org_id", data.OrgID),
			slog.Int64("assessment_id", data.AssessmentID),
			slog.Uint64("testee_id", data.TesteeID),
			slog.String("reason", data.Reason),
			slog.Time("failed_at", data.FailedAt),
		)

		if deps.InternalClient != nil {
			assessmentID, err := safeconv.Int64ToUint64(data.AssessmentID)
			if err != nil {
				return fmt.Errorf("invalid assessment id in failed event: %w", err)
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
				return fmt.Errorf("failed to project assessment failed behavior: %w", err)
			}
		}

		return nil
	}
}
