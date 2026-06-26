package result

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
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
	if n.waiterRegistry == nil || outcome.Assessment == nil || outcome.Execution == nil {
		return
	}
	assessmentID := outcome.Assessment.ID().Uint64()
	var totalScore float64
	if outcome.Execution.Primary != nil {
		totalScore = outcome.Execution.Primary.Value
	}
	riskLevelStr := string(assessment.RiskLevelNone)
	if outcome.Execution.Level != nil && outcome.Execution.Level.Code != "" {
		riskLevelStr = outcome.Execution.Level.Code
	}
	summary := evaluationwaiter.StatusSummary{
		Status:     "interpreted",
		TotalScore: &totalScore,
		RiskLevel:  &riskLevelStr,
		UpdatedAt:  time.Now().Unix(),
	}
	n.waiterRegistry.Notify(ctx, assessmentID, summary)

	logger.L(ctx).Debugw("notified waiters for assessment",
		"assessment_id", assessmentID,
		"waiter_count", n.waiterRegistry.GetWaiterCount(assessmentID),
	)
}
