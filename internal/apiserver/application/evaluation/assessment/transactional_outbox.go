package assessment

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

func saveAssessmentAndStageEvents(
	ctx context.Context,
	repo domainAssessment.Repository,
	txRunner apptransaction.Runner,
	stager EventStager,
	a *domainAssessment.Assessment,
	additional []event.DomainEvent,
) error {
	if txRunner == nil || stager == nil {
		return errors.WithCode(errorCode.ErrModuleInitializationFailed, "assessment transactional outbox requires transaction runner and event stager")
	}
	if a == nil {
		return nil
	}

	err := txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := repo.Save(txCtx, a); err != nil {
			return err
		}
		eventsToStage := make([]event.DomainEvent, 0, len(a.Events())+len(additional))
		eventsToStage = append(eventsToStage, a.Events()...)
		eventsToStage = append(eventsToStage, additional...)
		if len(eventsToStage) == 0 {
			return nil
		}
		return stager.Stage(txCtx, eventsToStage...)
	})
	if err != nil {
		return err
	}
	a.ClearEvents()
	return nil
}
