package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

func init() {
	Register("assessment_submitted_handler", func(deps *Dependencies) HandlerFunc {
		return handleAssessmentSubmitted(deps)
	})
	Register("assessment_interpreted_handler", func(deps *Dependencies) HandlerFunc {
		return handleAssessmentInterpreted(deps)
	})
	Register("assessment_failed_handler", func(deps *Dependencies) HandlerFunc {
		return handleAssessmentFailed(deps)
	})
}

// ==================== Payload 定义 ====================

// AssessmentSubmittedPayload 测评提交事件数据
type AssessmentSubmittedPayload struct {
	AssessmentID      int64     `json:"assessment_id"`
	TesteeID          uint64    `json:"testee_id"`
	QuestionnaireCode string    `json:"questionnaire_code"`
	QuestionnaireVer  string    `json:"questionnaire_version"`
	AnswerSheetID     string    `json:"answersheet_id"`
	ScaleCode         string    `json:"scale_code,omitempty"`
	ScaleVersion      string    `json:"scale_version,omitempty"`
	SubmittedAt       time.Time `json:"submitted_at"`
}

// NeedsEvaluation 是否需要评估（有量表才需要）
func (p AssessmentSubmittedPayload) NeedsEvaluation() bool {
	return p.ScaleCode != ""
}

// AssessmentInterpretedPayload 测评解读完成事件数据
type AssessmentInterpretedPayload struct {
	AssessmentID  int64     `json:"assessment_id"`
	TesteeID      uint64    `json:"testee_id"`
	ScaleCode     string    `json:"scale_code"`
	ScaleVersion  string    `json:"scale_version"`
	TotalScore    float64   `json:"total_score"`
	RiskLevel     string    `json:"risk_level"`
	InterpretedAt time.Time `json:"interpreted_at"`
}

// IsHighRisk 是否高风险
func (p AssessmentInterpretedPayload) IsHighRisk() bool {
	return p.RiskLevel == "high" || p.RiskLevel == "critical"
}

// AssessmentFailedPayload 测评失败事件数据
type AssessmentFailedPayload struct {
	AssessmentID int64     `json:"assessment_id"`
	TesteeID     uint64    `json:"testee_id"`
	Reason       string    `json:"reason"`
	FailedAt     time.Time `json:"failed_at"`
}

// ==================== Handler 实现 ====================

// handleAssessmentSubmitted 处理测评提交事件
// 业务逻辑：
// 1. 解析测评提交事件
// 2. 检查是否需要评估（有关联量表）
// 3. 调用 InternalClient 执行评估
func handleAssessmentSubmitted(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data AssessmentSubmittedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse assessment submitted event: %w", err)
		}

		deps.Logger.Info("processing assessment submitted",
			slog.String("event_id", env.ID),
			slog.Int64("assessment_id", data.AssessmentID),
			slog.Uint64("testee_id", data.TesteeID),
			slog.String("questionnaire_code", data.QuestionnaireCode),
			slog.String("answersheet_id", data.AnswerSheetID),
			slog.String("scale_code", data.ScaleCode),
			slog.Bool("needs_evaluation", data.NeedsEvaluation()),
		)

		// 如果没有关联量表，无需评估
		if !data.NeedsEvaluation() {
			deps.Logger.Info("assessment does not need evaluation (no scale)",
				slog.Int64("assessment_id", data.AssessmentID),
			)
			return nil
		}

		// 检查 InternalClient 是否可用
		if deps.InternalClient == nil {
			deps.Logger.Warn("InternalClient is not available, skipping evaluation",
				slog.Int64("assessment_id", data.AssessmentID),
			)
			return nil
		}

		// 调用 InternalClient 执行评估
		resp, err := deps.InternalClient.EvaluateAssessment(ctx, uint64(data.AssessmentID))
		if err != nil {
			deps.Logger.Error("failed to evaluate assessment",
				slog.Int64("assessment_id", data.AssessmentID),
				slog.String("error", err.Error()),
			)
			return fmt.Errorf("failed to evaluate assessment: %w", err)
		}

		deps.Logger.Info("assessment evaluation completed",
			slog.Int64("assessment_id", data.AssessmentID),
			slog.Bool("success", resp.Success),
			slog.String("status", resp.Status),
			slog.Float64("total_score", resp.TotalScore),
			slog.String("risk_level", resp.RiskLevel),
			slog.String("message", resp.Message),
		)

		return nil
	}
}

// handleAssessmentInterpreted 处理测评解读完成事件
// 业务逻辑：
// 1. 检查是否高风险
// 2. 发送预警通知（如有必要）
func handleAssessmentInterpreted(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data AssessmentInterpretedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse assessment interpreted event: %w", err)
		}

		deps.Logger.Info("processing assessment interpreted",
			slog.String("event_id", env.ID),
			slog.Int64("assessment_id", data.AssessmentID),
			slog.Float64("total_score", data.TotalScore),
			slog.String("risk_level", data.RiskLevel),
			slog.Bool("is_high_risk", data.IsHighRisk()),
		)

		// 高风险预警
		if data.IsHighRisk() {
			deps.Logger.Warn("HIGH RISK ALERT",
				slog.Int64("assessment_id", data.AssessmentID),
				slog.Uint64("testee_id", data.TesteeID),
				slog.String("risk_level", data.RiskLevel),
				slog.Float64("total_score", data.TotalScore),
			)
			// TODO: 发送预警通知（可以调用通知服务）
		}

		return nil
	}
}

// handleAssessmentFailed 处理测评失败事件
func handleAssessmentFailed(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data AssessmentFailedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse assessment failed event: %w", err)
		}

		deps.Logger.Error("assessment failed",
			slog.String("event_id", env.ID),
			slog.Int64("assessment_id", data.AssessmentID),
			slog.Uint64("testee_id", data.TesteeID),
			slog.String("reason", data.Reason),
			slog.Time("failed_at", data.FailedAt),
		)

		// TODO: 发送监控告警

		return nil
	}
}
