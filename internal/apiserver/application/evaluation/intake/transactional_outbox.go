package intake

import (
	"context"
	"time"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// EventStager 事件阶段器
// 行为者：测评提交服务
// 职责：阶段测评事件
// 变更来源：测评提交服务
type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

// saveAssessmentAndStageEvents 保存测评并阶段事件
// 场景：保存测评并阶段事件
func saveAssessmentAndStageEvents(
	ctx context.Context,
	repo domainAssessment.Repository,
	txRunner apptransaction.Runner,
	stager EventStager,
	a *domainAssessment.Assessment,
	postCommit appEventing.PostCommitDispatcher,
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
		eventsToStage := make([]event.DomainEvent, 0, len(a.Events()))
		eventsToStage = append(eventsToStage, a.Events()...)
		stagedEvents = eventsToStage
		if len(eventsToStage) == 0 {
			return nil
		}
		return stager.Stage(txCtx, eventsToStage...)
	})
	if err != nil {
		return err
	}
	if postCommit != nil && len(stagedEvents) > 0 {
		postCommit.AfterCommit(ctx, stagedEvents, time.Now())
	}
	a.ClearEvents()
	return nil
}
