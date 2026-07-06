package assessment

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/outboxpolicy"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// EventStager 事件阶段器
// 行为者：测评提交服务
// 职责：阶段测评事件
// 变更来源：测评提交服务
type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

// AdditionalEventBuilder builds outbox events after persistence assigns aggregate IDs.
type AdditionalEventBuilder func(a *domainAssessment.Assessment) []event.DomainEvent

// saveAssessmentAndStageEvents 保存测评并阶段事件
// 场景：保存测评并阶段事件
func saveAssessmentAndStageEvents(
	ctx context.Context,
	repo domainAssessment.Repository,
	txRunner apptransaction.Runner,
	stager EventStager,
	a *domainAssessment.Assessment,
	additional AdditionalEventBuilder,
	immediate *appEventing.ImmediateDispatcher,
) error {
	if txRunner == nil || stager == nil {
		return evalerrors.ModuleNotConfigured("assessment transactional outbox requires transaction runner and event stager")
	}
	if a == nil {
		return nil
	}

	var stagedEvents []event.DomainEvent
	err := txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := repo.Save(txCtx, a); err != nil {
			return err
		}
		var extra []event.DomainEvent
		if additional != nil {
			extra = additional(a)
		}
		eventsToStage := make([]event.DomainEvent, 0, len(a.Events())+len(extra))
		eventsToStage = append(eventsToStage, a.Events()...)
		eventsToStage = append(eventsToStage, extra...)
		eventsToStage = outboxpolicy.Filter(eventsToStage)
		stagedEvents = eventsToStage
		if len(eventsToStage) == 0 {
			return nil
		}
		return stager.Stage(txCtx, eventsToStage...)
	})
	if err != nil {
		return err
	}
	if immediate != nil && len(stagedEvents) > 0 {
		immediate.TryDispatchAfterCommit(ctx, stagedEvents)
	}
	a.ClearEvents()
	return nil
}
