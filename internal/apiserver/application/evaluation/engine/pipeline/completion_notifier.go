package pipeline

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
)

type CompletionNotifier interface {
	NotifyCompletion(ctx context.Context, evalCtx *Context)
}

type waiterCompletionNotifier struct {
	waiterRegistry evaluationwaiter.Notifier
}

func NewWaiterCompletionNotifier(waiterRegistry evaluationwaiter.Notifier) CompletionNotifier {
	return waiterCompletionNotifier{waiterRegistry: waiterRegistry}
}

func (n waiterCompletionNotifier) NotifyCompletion(ctx context.Context, evalCtx *Context) {
	if n.waiterRegistry == nil || evalCtx == nil || evalCtx.EvaluationResult == nil || evalCtx.Assessment == nil {
		return
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
	n.waiterRegistry.Notify(ctx, assessmentID, summary)

	logger.L(ctx).Debugw("notified waiters for assessment",
		"assessment_id", assessmentID,
		"waiter_count", n.waiterRegistry.GetWaiterCount(assessmentID),
	)
}
