package evaluation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	appeventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	evaluationevents "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/events"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type PromptEvaluationV2EventFactory interface {
	PromptEvaluationStepV2(*domainevaluation.PromptEvaluationEvidenceV2, domainevaluation.EvidenceNextAction, string, time.Time) (event.DomainEvent, error)
}

// DurableCommitterV2 keeps append-only execution evidence and the next durable
// wake-up in one transaction. Provider calls are always outside this boundary.
type DurableCommitterV2 struct {
	tx                            apptransaction.Runner
	repository                    domainevaluation.EvidenceV2Repository
	events                        PromptEvaluationV2EventFactory
	stager                        PromptEvaluationEventStager
	postCommit                    appeventing.PostCommitDispatcher
	capacity                      domainevaluation.CapacityRepository
	dailyProviderInvocationBudget int
	now                           func() time.Time
}

func NewDurableCommitterV2(
	tx apptransaction.Runner,
	repository domainevaluation.EvidenceV2Repository,
	events PromptEvaluationV2EventFactory,
	stager PromptEvaluationEventStager,
	postCommit appeventing.PostCommitDispatcher,
	capacity domainevaluation.CapacityRepository,
	dailyProviderInvocationBudget int,
	now func() time.Time,
) (*DurableCommitterV2, error) {
	if tx == nil || repository == nil || events == nil || stager == nil || postCommit == nil || capacity == nil || dailyProviderInvocationBudget < 1 {
		return nil, fmt.Errorf("AI explanation Prompt evaluation v2 durable commit dependencies are required")
	}
	if now == nil {
		now = time.Now
	}
	return &DurableCommitterV2{
		tx: tx, repository: repository, events: events, stager: stager, postCommit: postCommit,
		capacity: capacity, dailyProviderInvocationBudget: dailyProviderInvocationBudget, now: now,
	}, nil
}

func (c *DurableCommitterV2) CommitStartV2(ctx context.Context, value *domainevaluation.PromptEvaluationEvidenceV2) error {
	if c == nil || value == nil || value.Status != domainevaluation.EvidenceStatusCollecting || value.Execution() != nil ||
		value.Audit.OrganizationID <= 0 || strings.TrimSpace(value.Audit.RequestedBy) == "" || strings.TrimSpace(value.Audit.RequestReason) == "" {
		return fmt.Errorf("audited collecting AI explanation Prompt evaluation v2 is required")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	action, err := value.NextAction()
	if err != nil || action.Kind != domainevaluation.EvidenceNextActionGeneration || action.Resume {
		return fmt.Errorf("AI explanation Prompt evaluation v2 first generation action is required")
	}
	reservedCalls := value.ExecutionPolicy.WorstCaseProviderCalls()
	if reservedCalls < 1 || c.dailyProviderInvocationBudget < reservedCalls {
		return fmt.Errorf("AI explanation Prompt evaluation v2 daily Provider invocation budget is invalid")
	}
	at := c.now().UTC()
	budgetDay := domainevaluation.UTCBudgetDay(at)
	if err := c.capacity.EnsureDailyBucket(ctx, value.Audit.OrganizationID, budgetDay, at); err != nil {
		return err
	}
	eventRecord, err := c.nextEvent(value, action, at)
	if err != nil {
		return err
	}
	if err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.capacity.ReserveDailyProviderInvocations(txCtx, domainevaluation.DailyCapacityReservation{
			RunID: value.RunID, OrgID: value.Audit.OrganizationID, RequestedBy: value.Audit.RequestedBy,
			BudgetDay: budgetDay, ProviderInvocations: reservedCalls,
			DailyLimit: c.dailyProviderInvocationBudget, ReservedAt: at,
		}); err != nil {
			return err
		}
		if err := c.repository.CreateEvidenceV2(txCtx, value); err != nil {
			return err
		}
		return c.stager.Stage(txCtx, eventRecord)
	}); err != nil {
		return err
	}
	c.postCommit.AfterCommit(ctx, []event.DomainEvent{eventRecord}, eventRecord.OccurredAt())
	return nil
}

func (c *DurableCommitterV2) CommitGenerationV2(
	ctx context.Context,
	runID meta.ID,
	command CompleteGenerationV2Command,
) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return c.commitTerminalV2(ctx, runID, func(value *domainevaluation.PromptEvaluationEvidenceV2) error {
		return value.CompleteGenerationExecution(command.Owner, command.CandidateID, command.Assertions, command.Execution)
	})
}

func (c *DurableCommitterV2) CommitSemanticV2(
	ctx context.Context,
	runID meta.ID,
	owner string,
	execution domainevaluation.SemanticEvaluationExecution,
) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return c.commitTerminalV2(ctx, runID, func(value *domainevaluation.PromptEvaluationEvidenceV2) error {
		return value.CompleteSemanticExecution(owner, execution)
	})
}

// CommitResultUnknownResolutionV2 keeps the audited manual decision and the
// replacement wake-up atomic. Authorizing a replacement without staging the
// next durable action would otherwise leave a collecting Run permanently idle.
func (c *DurableCommitterV2) CommitResultUnknownResolutionV2(
	ctx context.Context,
	runID meta.ID,
	resolution domainevaluation.ResultUnknownResolution,
) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return c.commitTerminalV2(ctx, runID, func(value *domainevaluation.PromptEvaluationEvidenceV2) error {
		return value.ResolveResultUnknown(resolution)
	})
}

func (c *DurableCommitterV2) commitTerminalV2(
	ctx context.Context,
	runID meta.ID,
	mutation func(*domainevaluation.PromptEvaluationEvidenceV2) error,
) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	if c == nil || runID.IsZero() || mutation == nil {
		return nil, fmt.Errorf("AI explanation Prompt evaluation v2 durable completion is invalid")
	}
	value, err := c.repository.FindEvidenceV2ByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	expectedVersion := value.Version()
	if err := mutation(value); err != nil {
		return nil, err
	}
	action, err := value.NextAction()
	if err != nil {
		return nil, err
	}
	var nextEvent event.DomainEvent
	switch action.Kind {
	case domainevaluation.EvidenceNextActionGeneration, domainevaluation.EvidenceNextActionSemantic:
		nextEvent, err = c.nextEvent(value, action, c.now().UTC())
		if err != nil {
			return nil, err
		}
	case domainevaluation.EvidenceNextActionPreflight:
		return nil, fmt.Errorf("AI explanation Prompt evaluation v2 completion returned to preflight")
	}
	if err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.repository.SaveEvidenceV2(txCtx, value, expectedVersion); err != nil {
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
	return value, nil
}

func (c *DurableCommitterV2) nextEvent(
	value *domainevaluation.PromptEvaluationEvidenceV2,
	action domainevaluation.EvidenceNextAction,
	at time.Time,
) (event.DomainEvent, error) {
	eventID := evaluationevents.PromptEvaluationStepV2EventID(value.RunID.String(), action)
	return c.events.PromptEvaluationStepV2(value, action, eventID, at)
}

var _ PromptEvaluationV2EventFactory = evaluationevents.Factory{}
