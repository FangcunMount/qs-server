package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// AssessmentFailedHandler 处理测评失败事件
// 职责：
// - 记录失败日志
// - 更新监控指标
// - 可选：发送失败通知
type AssessmentFailedHandler struct {
	*BaseHandler
	logger *slog.Logger
}

// NewAssessmentFailedHandler 创建测评失败事件处理器
func NewAssessmentFailedHandler(logger *slog.Logger) *AssessmentFailedHandler {
	return &AssessmentFailedHandler{
		BaseHandler: NewBaseHandler("assessment.failed", "assessment_failed_handler"),
		logger:      logger,
	}
}

// Handle 处理测评失败事件
func (h *AssessmentFailedHandler) Handle(ctx context.Context, payload []byte) error {
	var event assessment.AssessmentFailedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		h.logger.Error("failed to unmarshal assessment failed event",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Error("assessment failed",
		slog.String("handler", h.Name()),
		slog.String("event_id", event.EventID()),
		slog.String("assessment_id", fmt.Sprintf("%d", event.AssessmentID())),
		slog.String("testee_id", fmt.Sprintf("%d", event.TesteeID())),
		slog.String("reason", event.Reason()),
		slog.Time("failed_at", event.FailedAt()),
	)

	// 1. 更新监控指标
	if err := h.updateMetrics(ctx, &event); err != nil {
		h.logger.Warn("failed to update metrics",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
	}

	// 2. 发送告警（如果失败率过高）
	if err := h.checkAndAlert(ctx, &event); err != nil {
		h.logger.Warn("failed to check and alert",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
	}

	h.logger.Info("assessment failed event processed",
		slog.String("handler", h.Name()),
		slog.String("assessment_id", fmt.Sprintf("%d", event.AssessmentID())),
	)

	return nil
}

// updateMetrics 更新监控指标
func (h *AssessmentFailedHandler) updateMetrics(ctx context.Context, event *assessment.AssessmentFailedEvent) error {
	h.logger.Debug("updating failure metrics",
		slog.String("assessment_id", fmt.Sprintf("%d", event.AssessmentID())),
	)

	// TODO: 更新 Prometheus 指标
	// metrics.AssessmentFailureCounter.Inc()
	// metrics.AssessmentFailureByReason.WithLabelValues(event.Reason()).Inc()

	return nil
}

// checkAndAlert 检查失败率并告警
func (h *AssessmentFailedHandler) checkAndAlert(ctx context.Context, event *assessment.AssessmentFailedEvent) error {
	// TODO: 实现失败率检查
	// 如果短时间内失败率过高，发送运维告警
	//
	// failureRate := h.getRecentFailureRate(ctx)
	// if failureRate > 0.1 { // 失败率超过 10%
	//     h.alertClient.SendOpsAlert(ctx, &OpsAlertRequest{
	//         Type:     "high_failure_rate",
	//         Severity: "critical",
	//         Message:  fmt.Sprintf("Assessment failure rate is %.2f%%", failureRate*100),
	//     })
	// }

	return nil
}
