package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// AssessmentInterpretedHandler 处理测评解读完成事件
// 职责：
// - 发送"报告已生成"通知给受试者
// - 对高风险案例发送预警给相关人员
// - 更新统计数据
type AssessmentInterpretedHandler struct {
	*BaseHandler
	logger *slog.Logger
	// TODO: 注入通知服务、预警服务客户端
}

// NewAssessmentInterpretedHandler 创建测评解读完成事件处理器
func NewAssessmentInterpretedHandler(logger *slog.Logger) *AssessmentInterpretedHandler {
	return &AssessmentInterpretedHandler{
		BaseHandler: NewBaseHandler("assessment.interpreted", "assessment_interpreted_handler"),
		logger:      logger,
	}
}

// Handle 处理测评解读完成事件
func (h *AssessmentInterpretedHandler) Handle(ctx context.Context, payload []byte) error {
	var event assessment.AssessmentInterpretedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		h.logger.Error("failed to unmarshal assessment interpreted event",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Info("processing assessment interpreted event",
		slog.String("handler", h.Name()),
		slog.String("event_id", event.EventID()),
		slog.String("assessment_id", fmt.Sprintf("%d", event.AssessmentID())),
		slog.String("testee_id", fmt.Sprintf("%d", event.TesteeID())),
		slog.Float64("total_score", event.TotalScore()),
		slog.String("risk_level", string(event.RiskLevel())),
	)

	// 1. 发送"报告已生成"通知
	if err := h.sendReportReadyNotification(ctx, &event); err != nil {
		h.logger.Error("failed to send report ready notification",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		// 通知失败不阻塞流程，继续处理
	}

	// 2. 高风险预警
	if event.IsHighRisk() {
		if err := h.sendHighRiskAlert(ctx, &event); err != nil {
			h.logger.Error("failed to send high risk alert",
				slog.String("handler", h.Name()),
				slog.String("error", err.Error()),
			)
			// 预警失败需要重试
			return err
		}
	}

	// 3. 更新统计数据
	if err := h.updateStatistics(ctx, &event); err != nil {
		h.logger.Warn("failed to update statistics",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		// 统计失败不阻塞流程
	}

	h.logger.Info("assessment interpreted event processed successfully",
		slog.String("handler", h.Name()),
		slog.String("assessment_id", fmt.Sprintf("%d", event.AssessmentID())),
	)

	return nil
}

// sendReportReadyNotification 发送报告就绪通知
func (h *AssessmentInterpretedHandler) sendReportReadyNotification(ctx context.Context, event *assessment.AssessmentInterpretedEvent) error {
	h.logger.Debug("sending report ready notification",
		slog.String("testee_id", fmt.Sprintf("%d", event.TesteeID())),
	)

	// TODO: 调用通知服务
	// return h.notificationClient.SendNotification(ctx, &SendNotificationRequest{
	//     UserID:   event.TesteeID(),
	//     Type:     "report_ready",
	//     Title:    "您的测评报告已生成",
	//     Content:  "点击查看详细报告",
	//     Metadata: map[string]string{"assessment_id": event.AssessmentID()},
	// })

	return nil
}

// sendHighRiskAlert 发送高风险预警
func (h *AssessmentInterpretedHandler) sendHighRiskAlert(ctx context.Context, event *assessment.AssessmentInterpretedEvent) error {
	h.logger.Warn("sending high risk alert",
		slog.String("testee_id", fmt.Sprintf("%d", event.TesteeID())),
		slog.String("risk_level", string(event.RiskLevel())),
		slog.Float64("total_score", event.TotalScore()),
	)

	// TODO: 调用预警服务
	// return h.alertClient.SendAlert(ctx, &SendAlertRequest{
	//     Type:       "high_risk_assessment",
	//     Severity:   "high",
	//     TesteeID:   event.TesteeID(),
	//     AssessmentID: event.AssessmentID(),
	//     RiskLevel:  event.RiskLevel(),
	//     TotalScore: event.TotalScore(),
	// })

	return nil
}

// updateStatistics 更新统计数据
func (h *AssessmentInterpretedHandler) updateStatistics(ctx context.Context, event *assessment.AssessmentInterpretedEvent) error {
	h.logger.Debug("updating statistics",
		slog.String("assessment_id", fmt.Sprintf("%d", event.AssessmentID())),
	)

	// TODO: 调用统计服务
	// return h.statisticsClient.IncrementAssessmentCount(ctx, &IncrementRequest{
	//     ScaleCode:  event.MedicalScaleRef().Code(),
	//     RiskLevel:  event.RiskLevel(),
	//     Date:       event.InterpretedAt(),
	// })

	return nil
}
