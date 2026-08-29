package evaluation

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	appeventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	evaluationevents "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/events"
)

type PromptEvaluationRecheckEventFactory interface {
	PromptEvaluationRecheck(*domainevaluation.PromptEvaluationRecheck, string, time.Time) (event.DomainEvent, error)
}

// RecheckCommitter atomically reserves the bounded two-call cost, creates the
// diagnostic aggregate and stages its one durable wake-up.
type RecheckCommitter struct {
	tx         apptransaction.Runner
	repository domainevaluation.RecheckRepository
	events     PromptEvaluationRecheckEventFactory
	stager     PromptEvaluationEventStager
	postCommit appeventing.PostCommitDispatcher
	capacity   domainevaluation.CapacityRepository
	dailyLimit int
	now        func() time.Time
}

func NewRecheckCommitter(
	tx apptransaction.Runner,
	repository domainevaluation.RecheckRepository,
	events PromptEvaluationRecheckEventFactory,
	stager PromptEvaluationEventStager,
	postCommit appeventing.PostCommitDispatcher,
	capacity domainevaluation.CapacityRepository,
	dailyLimit int,
	now func() time.Time,
) (*RecheckCommitter, error) {
	if tx == nil || repository == nil || events == nil || stager == nil || postCommit == nil || capacity == nil ||
		dailyLimit < domainevaluation.RecheckProviderInvocationsV1 {
		return nil, fmt.Errorf("AI explanation attempt recheck durable commit dependencies are required")
	}
	if now == nil {
		now = time.Now
	}
	return &RecheckCommitter{tx: tx, repository: repository, events: events, stager: stager, postCommit: postCommit, capacity: capacity, dailyLimit: dailyLimit, now: now}, nil
}

func (c *RecheckCommitter) CommitRecheckStart(ctx context.Context, value *domainevaluation.PromptEvaluationRecheck) error {
	if c == nil || value == nil || value.Status() != domainevaluation.RecheckStatusQueued {
		return fmt.Errorf("queued AI explanation attempt recheck is required")
	}
	at := c.now().UTC()
	budgetDay := domainevaluation.UTCBudgetDay(at)
	if err := c.capacity.EnsureDailyBucket(ctx, value.RequestedOrgID(), budgetDay, at); err != nil {
		return err
	}
	eventID := evaluationevents.PromptEvaluationRecheckEventID(value.ID().String())
	eventRecord, err := c.events.PromptEvaluationRecheck(value, eventID, at)
	if err != nil {
		return err
	}
	if err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.capacity.ReserveDailyProviderInvocations(txCtx, domainevaluation.DailyCapacityReservation{
			RunID: value.ID(), OrgID: value.RequestedOrgID(), RequestedBy: value.RequestedBy(), BudgetDay: budgetDay,
			ProviderInvocations: domainevaluation.RecheckProviderInvocationsV1, DailyLimit: c.dailyLimit, ReservedAt: at,
		}); err != nil {
			return err
		}
		if err := c.repository.CreateRecheck(txCtx, value); err != nil {
			return err
		}
		return c.stager.Stage(txCtx, eventRecord)
	}); err != nil {
		return err
	}
	c.postCommit.AfterCommit(ctx, []event.DomainEvent{eventRecord}, eventRecord.OccurredAt())
	return nil
}

var _ PromptEvaluationRecheckEventFactory = evaluationevents.Factory{}
