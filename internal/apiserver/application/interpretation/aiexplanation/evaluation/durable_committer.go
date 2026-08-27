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
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type PromptEvaluationEventFactory interface {
	PromptEvaluationStep(*domainevaluation.PromptEvaluationRun, string, int, string, time.Time) (event.DomainEvent, error)
}

type PromptEvaluationEventStager interface {
	Stage(context.Context, ...event.DomainEvent) error
}

// DurableCommitter atomically binds the initial organization budget
// reservation and evidence run to its first wake-up, then each completed
// attempt to the next wake-up. Provider calls never execute in these
// transactions.
type DurableCommitter struct {
	tx                            apptransaction.Runner
	repository                    domainevaluation.Repository
	events                        PromptEvaluationEventFactory
	stager                        PromptEvaluationEventStager
	postCommit                    appeventing.PostCommitDispatcher
	capacity                      domainevaluation.CapacityRepository
	dailyProviderInvocationBudget int
	now                           func() time.Time
}

func NewDurableCommitter(
	tx apptransaction.Runner,
	repository domainevaluation.Repository,
	events PromptEvaluationEventFactory,
	stager PromptEvaluationEventStager,
	postCommit appeventing.PostCommitDispatcher,
	capacity domainevaluation.CapacityRepository,
	dailyProviderInvocationBudget int,
	now func() time.Time,
) (*DurableCommitter, error) {
	if tx == nil || repository == nil || events == nil || stager == nil || postCommit == nil || capacity == nil {
		return nil, fmt.Errorf("AI explanation Prompt evaluation durable commit dependencies are required")
	}
	if dailyProviderInvocationBudget < MaxProviderInvocationsV1 || dailyProviderInvocationBudget%MaxProviderInvocationsV1 != 0 {
		return nil, fmt.Errorf("AI explanation Prompt evaluation daily Provider invocation budget is invalid")
	}
	if now == nil {
		now = time.Now
	}
	return &DurableCommitter{
		tx: tx, repository: repository, events: events, stager: stager, postCommit: postCommit,
		capacity: capacity, dailyProviderInvocationBudget: dailyProviderInvocationBudget, now: now,
	}, nil
}

func (c *DurableCommitter) CommitStart(ctx context.Context, runRecord *domainevaluation.PromptEvaluationRun) error {
	if c == nil || runRecord == nil || runRecord.Status() != domainevaluation.StatusCollecting || runRecord.Execution() != nil ||
		runRecord.RequestedOrgID() <= 0 || runRecord.RequestedBy() == "" || runRecord.RequestReason() == "" {
		err := fmt.Errorf("audited collecting AI explanation Prompt evaluation is required")
		observePromptEvaluationStartAdmission(err)
		return err
	}
	caseID, attempt, ok := runRecord.NextPendingGenerationAttempt()
	if !ok {
		err := fmt.Errorf("AI explanation Prompt evaluation first generation attempt is required")
		observePromptEvaluationStartAdmission(err)
		return err
	}
	at := c.now().UTC()
	budgetDay := domainevaluation.UTCBudgetDay(at)
	if err := c.capacity.EnsureDailyBucket(ctx, runRecord.RequestedOrgID(), budgetDay, at); err != nil {
		observePromptEvaluationStartAdmission(err)
		return err
	}
	eventRecord, err := c.nextEvent(runRecord, caseID, attempt, at)
	if err != nil {
		observePromptEvaluationStartAdmission(err)
		return err
	}
	if err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.capacity.ReserveDailyProviderInvocations(txCtx, domainevaluation.DailyCapacityReservation{
			RunID: runRecord.ID(), OrgID: runRecord.RequestedOrgID(), RequestedBy: runRecord.RequestedBy(),
			BudgetDay: budgetDay, ProviderInvocations: MaxProviderInvocationsV1,
			DailyLimit: c.dailyProviderInvocationBudget, ReservedAt: at,
		}); err != nil {
			return err
		}
		if err := c.repository.Create(txCtx, runRecord); err != nil {
			return err
		}
		return c.stager.Stage(txCtx, eventRecord)
	}); err != nil {
		observePromptEvaluationStartAdmission(err)
		return err
	}
	observePromptEvaluationStartAdmission(nil)
	c.postCommit.AfterCommit(ctx, []event.DomainEvent{eventRecord}, eventRecord.OccurredAt())
	return nil
}

func (c *DurableCommitter) CommitAttempt(
	ctx context.Context,
	runID meta.ID,
	owner string,
	attemptRecord domainevaluation.AttemptRecord,
) (*domainevaluation.PromptEvaluationRun, error) {
	if c == nil || runID.IsZero() {
		return nil, fmt.Errorf("AI explanation Prompt evaluation durable completion is invalid")
	}
	runRecord, err := c.repository.FindByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	expectedVersion := runRecord.Version()
	if err := runRecord.CompleteAttemptExecution(owner, attemptRecord); err != nil {
		return nil, err
	}
	var nextEvent event.DomainEvent
	if caseID, attempt, pending := runRecord.NextPendingGenerationAttempt(); pending {
		nextEvent, err = c.nextEvent(runRecord, caseID, attempt, c.now().UTC())
		if err != nil {
			return nil, err
		}
	} else if err := runRecord.CloseCollection(c.now().UTC()); err != nil {
		return nil, err
	}
	if err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.repository.Save(txCtx, runRecord, expectedVersion); err != nil {
			return err
		}
		if nextEvent != nil {
			return c.stager.Stage(txCtx, nextEvent)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if nextEvent != nil {
		c.postCommit.AfterCommit(ctx, []event.DomainEvent{nextEvent}, nextEvent.OccurredAt())
	}
	return runRecord, nil
}

// CommitRecovery atomically appends an operator audit record and a new wake-up
// event. It never clears or rewrites an existing dispatch checkpoint.
func (c *DurableCommitter) CommitRecovery(
	ctx context.Context,
	runID meta.ID,
	requestID, actor, reason string,
) (*domainevaluation.PromptEvaluationRun, error) {
	if c == nil || runID.IsZero() {
		return nil, fmt.Errorf("AI explanation Prompt evaluation recovery is invalid")
	}
	runRecord, err := c.repository.FindByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	expectedVersion := runRecord.Version()
	at := c.now().UTC()
	caseID, attempt, err := runRecord.RequestRecovery(requestID, actor, reason, at)
	if err != nil {
		return nil, err
	}
	eventID := evaluationevents.PromptEvaluationRecoveryEventID(runID.String(), requestID)
	eventRecord, err := c.events.PromptEvaluationStep(runRecord, caseID, attempt, eventID, at)
	if err != nil {
		return nil, err
	}
	if err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.repository.Save(txCtx, runRecord, expectedVersion); err != nil {
			return err
		}
		return c.stager.Stage(txCtx, eventRecord)
	}); err != nil {
		return nil, err
	}
	c.postCommit.AfterCommit(ctx, []event.DomainEvent{eventRecord}, eventRecord.OccurredAt())
	return runRecord, nil
}

// CommitExpiredPreparationRecovery atomically rechecks an exact prepared
// invocation, appends its system audit record and stages a durable wake-up.
// The aggregate rejects dispatching checkpoints, and Save CAS closes the race
// between the recheck and commit.
func (c *DurableCommitter) CommitExpiredPreparationRecovery(
	ctx context.Context,
	runID meta.ID,
	invocationID string,
	observedLeaseExpiresAt time.Time,
	requestID, actor, reason string,
) (*domainevaluation.PromptEvaluationRun, error) {
	if c == nil || runID.IsZero() {
		return nil, fmt.Errorf("AI explanation Prompt evaluation prepared recovery is invalid")
	}
	runRecord, err := c.repository.FindByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	expectedVersion := runRecord.Version()
	at := c.now().UTC()
	caseID, attempt, err := runRecord.RequestExpiredPreparationRecovery(requestID, invocationID, observedLeaseExpiresAt, actor, reason, at)
	if err != nil {
		return nil, err
	}
	eventID := evaluationevents.PromptEvaluationRecoveryEventID(runID.String(), requestID)
	eventRecord, err := c.events.PromptEvaluationStep(runRecord, caseID, attempt, eventID, at)
	if err != nil {
		return nil, err
	}
	if err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.repository.Save(txCtx, runRecord, expectedVersion); err != nil {
			return err
		}
		return c.stager.Stage(txCtx, eventRecord)
	}); err != nil {
		return nil, err
	}
	c.postCommit.AfterCommit(ctx, []event.DomainEvent{eventRecord}, eventRecord.OccurredAt())
	return runRecord, nil
}

func (c *DurableCommitter) nextEvent(
	runRecord *domainevaluation.PromptEvaluationRun,
	caseID string,
	attempt int,
	at time.Time,
) (event.DomainEvent, error) {
	eventID := evaluationevents.PromptEvaluationStepEventID(runRecord.ID().String(), caseID, attempt)
	return c.events.PromptEvaluationStep(runRecord, caseID, attempt, eventID, at)
}

var _ PromptEvaluationEventFactory = evaluationevents.Factory{}
