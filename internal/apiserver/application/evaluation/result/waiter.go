package result

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
)

type CompletionNotifier interface {
	NotifyCompletion(ctx context.Context, outcome Outcome)
}

type waiterCompletionNotifier struct {
	waiterRegistry evaluationwaiter.Notifier
}

func NewWaiterCompletionNotifier(waiterRegistry evaluationwaiter.Notifier) CompletionNotifier {
	return waiterCompletionNotifier{waiterRegistry: waiterRegistry}
}

func (n waiterCompletionNotifier) NotifyCompletion(ctx context.Context, outcome Outcome) {
	if n.waiterRegistry == nil || outcome.Result == nil || outcome.Assessment == nil {
		return
	}
	result := outcome.Result
	assessmentID := outcome.Assessment.ID().Uint64()
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
