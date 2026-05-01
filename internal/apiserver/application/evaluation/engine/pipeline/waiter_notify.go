package pipeline

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
)

// WaiterNotifyHandler 只负责长轮询 waiter 的本地通知，不承担事件投递。
type WaiterNotifyHandler struct {
	*BaseHandler
	waiterRegistry evaluationwaiter.Notifier
}

// NewWaiterNotifyHandler 创建 waiter 通知处理器。
func NewWaiterNotifyHandler(waiterRegistry evaluationwaiter.Notifier) *WaiterNotifyHandler {
	return &WaiterNotifyHandler{
		BaseHandler:    NewBaseHandler("WaiterNotifyHandler"),
		waiterRegistry: waiterRegistry,
	}
}

// Handle 在评估成功后通知等待队列。
func (h *WaiterNotifyHandler) Handle(ctx context.Context, evalCtx *Context) error {
	if h.waiterRegistry == nil || evalCtx.EvaluationResult == nil || evalCtx.Assessment == nil {
		return h.Next(ctx, evalCtx)
	}

	result := evalCtx.EvaluationResult
	assessmentID := evalCtx.Assessment.ID().Uint64()
	riskLevelStr := string(result.RiskLevel)
	summary := evaluationwaiter.StatusSummary{
		Status:     "interpreted",
		TotalScore: &result.TotalScore,
		RiskLevel:  &riskLevelStr,
		UpdatedAt:  time.Now().Unix(),
	}
	h.waiterRegistry.Notify(ctx, assessmentID, summary)

	logger.L(ctx).Debugw("notified waiters for assessment",
		"assessment_id", assessmentID,
		"waiter_count", h.waiterRegistry.GetWaiterCount(assessmentID),
	)

	return h.Next(ctx, evalCtx)
}
