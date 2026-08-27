// Package persistence implements the atomic lifecycle boundaries required by
// manual AI explanation requests and asynchronous one-shot execution.
package persistence

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/event"
	appeventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainartifact "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

// EventFactory keeps wire-catalog details out of the lifecycle transaction.
// Every returned event must use the Generation ID as aggregate identity.
type EventFactory interface {
	Requested(*domaingeneration.AIExplanationGeneration) (event.DomainEvent, error)
	RetryRequested(*domaingeneration.AIExplanationGeneration, *domainrun.AIExplanationRun, domainrun.RetryAuthorization) (event.DomainEvent, error)
	LeaseRecoveryRequested(*domaingeneration.AIExplanationGeneration, *domainrun.AIExplanationRun, domainrun.RecoveryWakeup) (event.DomainEvent, error)
	Generated(*domaingeneration.AIExplanationGeneration, *domainrun.AIExplanationRun, *domainartifact.AIExplanationArtifact) (event.DomainEvent, error)
	Failed(*domaingeneration.AIExplanationGeneration, *domainrun.AIExplanationRun) (event.DomainEvent, error)
}

type Committer struct {
	tx             apptransaction.Runner
	generations    domaingeneration.Repository
	runs           domainrun.Repository
	artifacts      domainartifact.Repository
	capacity       domaingeneration.ParticipantCapacityRepository
	activeCapacity domaingeneration.ParticipantActiveCapacityRepository
	capacityPolicy domaingeneration.ParticipantCapacityPolicy
	events         EventFactory
	stager         EventStager
	postCommit     appeventing.PostCommitDispatcher
}

func NewCommitter(
	tx apptransaction.Runner,
	generations domaingeneration.Repository,
	runs domainrun.Repository,
	artifacts domainartifact.Repository,
	capacity domaingeneration.ParticipantCapacityRepository,
	activeCapacity domaingeneration.ParticipantActiveCapacityRepository,
	capacityPolicy domaingeneration.ParticipantCapacityPolicy,
	events EventFactory,
	stager EventStager,
	postCommit appeventing.PostCommitDispatcher,
) (*Committer, error) {
	if tx == nil || generations == nil || runs == nil || artifacts == nil || capacity == nil || activeCapacity == nil || events == nil || stager == nil || postCommit == nil {
		return nil, fmt.Errorf("AI explanation persistence committer dependencies are required")
	}
	if err := capacityPolicy.Validate(); err != nil {
		return nil, err
	}
	return &Committer{
		tx: tx, generations: generations, runs: runs, artifacts: artifacts, capacity: capacity, activeCapacity: activeCapacity, capacityPolicy: capacityPolicy,
		events: events, stager: stager, postCommit: postCommit,
	}, nil
}

var _ appport.RequestCommitter = (*Committer)(nil)
var _ appport.ExecutionCommitter = (*Committer)(nil)
var _ appport.RetryAuthorizationCommitter = (*Committer)(nil)

func (c *Committer) CommitRequested(ctx context.Context, generation *domaingeneration.AIExplanationGeneration) error {
	if generation == nil || generation.Status() != domaingeneration.StatusPending || generation.RequestedBy().Kind != "participant" {
		err := fmt.Errorf("pending participant AI explanation Generation is required")
		observeParticipantRequestAdmission(err)
		return err
	}
	association := generation.Association()
	reservedAt := generation.CreatedAt().UTC()
	reservation := domaingeneration.ParticipantDailyCapacityReservation{
		ReservationID: domaingeneration.ParticipantCapacityReservationID(generation.ID(), 1),
		GenerationID:  generation.ID(), Attempt: 1, Origin: retrygovernance.AttemptOriginInitial,
		OrgID: association.OrgID, UserID: generation.RequestedBy().ID,
		AssessmentID: association.AssessmentID, BudgetDay: domaingeneration.ParticipantUTCBudgetDay(reservedAt),
		ProviderInvocations: domaingeneration.ParticipantProviderInvocationsPerGenerationV1,
		Policy:              c.capacityPolicy, ReservedAt: reservedAt,
	}
	if err := reservation.Validate(); err != nil {
		observeParticipantRequestAdmission(err)
		return err
	}
	if err := c.capacity.EnsureParticipantDailyBucket(ctx, reservation.OrgID, reservation.BudgetDay, reservedAt); err != nil {
		observeParticipantRequestAdmission(err)
		return err
	}
	requested, err := c.events.Requested(generation)
	if err != nil {
		observeParticipantRequestAdmission(err)
		return err
	}
	if err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.capacity.ReserveParticipantDailyProviderInvocations(txCtx, reservation); err != nil {
			return err
		}
		if err := c.generations.Create(txCtx, generation); err != nil {
			return err
		}
		return c.stager.Stage(txCtx, requested)
	}); err != nil {
		observeParticipantRequestAdmission(err)
		return err
	}
	observeParticipantRequestAdmission(nil)
	c.postCommit.AfterCommit(ctx, []event.DomainEvent{requested}, requested.OccurredAt())
	return nil
}

func (c *Committer) CommitRetryAuthorization(
	ctx context.Context,
	generation *domaingeneration.AIExplanationGeneration,
	latest *domainrun.AIExplanationRun,
	authorization domainrun.RetryAuthorization,
) (*domainrun.AIExplanationRun, bool, error) {
	if generation == nil || latest == nil || generation.Status() != domaingeneration.StatusFailed || latest.Status() != domainrun.StatusFailed ||
		generation.LatestRunID() != latest.ID() || latest.GenerationID() != generation.ID() || authorization.ExpectedAttempt != latest.Attempt() {
		return nil, false, fmt.Errorf("failed AI explanation Generation and latest Run are required")
	}
	if err := authorization.Validate(); err != nil {
		return nil, false, err
	}
	retryEvent, err := c.events.RetryRequested(generation, latest, authorization)
	if err != nil {
		return nil, false, err
	}
	association := generation.Association()
	reservation := domaingeneration.ParticipantDailyCapacityReservation{
		ReservationID: domaingeneration.ParticipantCapacityReservationID(generation.ID(), authorization.NextAttempt),
		GenerationID:  generation.ID(), Attempt: authorization.NextAttempt, Origin: authorization.Origin,
		OrgID: association.OrgID, UserID: generation.RequestedBy().ID, AssessmentID: association.AssessmentID,
		BudgetDay:           domaingeneration.ParticipantUTCBudgetDay(authorization.AuthorizedAt),
		ProviderInvocations: domaingeneration.ParticipantProviderInvocationsPerAttemptV1,
		Policy:              c.capacityPolicy, ReservedAt: authorization.AuthorizedAt,
	}
	if err := reservation.Validate(); err != nil {
		return nil, false, err
	}
	if err := c.capacity.EnsureParticipantDailyBucket(ctx, reservation.OrgID, reservation.BudgetDay, reservation.ReservedAt); err != nil {
		return nil, false, err
	}
	authorizer, ok := c.runs.(domainrun.RetryAuthorizer)
	if !ok {
		return nil, false, fmt.Errorf("AI explanation Run repository does not support retry authorization")
	}
	var authorized *domainrun.AIExplanationRun
	created := false
	if err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var authorizeErr error
		authorized, created, authorizeErr = authorizer.AuthorizeRetry(txCtx, generation.ID(), authorization)
		if authorizeErr != nil || !created {
			return authorizeErr
		}
		if err := c.capacity.ReserveParticipantDailyProviderInvocations(txCtx, reservation); err != nil {
			return err
		}
		return c.stager.Stage(txCtx, retryEvent)
	}); err != nil {
		return nil, false, err
	}
	if created {
		c.postCommit.AfterCommit(ctx, []event.DomainEvent{retryEvent}, retryEvent.OccurredAt())
	}
	return authorized, created, nil
}

// CommitLeaseRecoveryWakeup records the exact expired lease and its durable
// outbox event in one transaction. It does not reclaim the lease, invoke the
// Provider, reserve budget, or create a new Run.
func (c *Committer) CommitLeaseRecoveryWakeup(
	ctx context.Context,
	generation *domaingeneration.AIExplanationGeneration,
	run *domainrun.AIExplanationRun,
	wakeup domainrun.RecoveryWakeup,
) (bool, error) {
	if generation == nil || run == nil || generation.Status() != domaingeneration.StatusGenerating ||
		run.Status() != domainrun.StatusRunning || generation.LatestRunID() != run.ID() ||
		run.GenerationID() != generation.ID() {
		return false, domainrun.ErrRecoveryNotAllowed
	}
	if err := wakeup.Validate(); err != nil {
		return false, err
	}
	recoveryEvent, err := c.events.LeaseRecoveryRequested(generation, run, wakeup)
	if err != nil {
		return false, err
	}
	scheduler, ok := c.runs.(domainrun.RecoveryWakeupScheduler)
	if !ok {
		return false, fmt.Errorf("AI explanation Run repository does not support durable lease recovery wake-up")
	}
	created := false
	if err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, scheduled, scheduleErr := scheduler.ScheduleRecoveryWakeup(txCtx, run.ID(), wakeup)
		if scheduleErr != nil || !scheduled {
			return scheduleErr
		}
		created = true
		return c.stager.Stage(txCtx, recoveryEvent)
	}); err != nil {
		return false, err
	}
	if created {
		c.postCommit.AfterCommit(ctx, []event.DomainEvent{recoveryEvent}, recoveryEvent.OccurredAt())
	}
	return created, nil
}

func (c *Committer) CommitStart(ctx context.Context, generation *domaingeneration.AIExplanationGeneration, run *domainrun.AIExplanationRun, expectedVersion uint64) error {
	if generation == nil || run == nil || generation.Status() != domaingeneration.StatusGenerating || run.Status() != domainrun.StatusRunning || run.InvocationPhase() != domainrun.InvocationPhasePrepared || generation.LatestRunID() != run.ID() || run.GenerationID() != generation.ID() {
		return fmt.Errorf("prepared AI explanation Generation and Run are required")
	}
	startedAt := run.StartedAt()
	if startedAt == nil {
		return fmt.Errorf("prepared AI explanation Run start time is required")
	}
	association := generation.Association()
	slot := domaingeneration.ParticipantActiveSlot{
		GenerationID: generation.ID(), RunID: run.ID(), OrgID: association.OrgID,
		UserID: generation.RequestedBy().ID, AssessmentID: association.AssessmentID,
		Policy: c.capacityPolicy, AcquiredAt: *startedAt,
	}
	if err := slot.Validate(); err != nil {
		observeParticipantExecutionAdmission(err)
		return err
	}
	if err := c.activeCapacity.EnsureParticipantActiveBucket(ctx, slot.OrgID, slot.AcquiredAt); err != nil {
		observeParticipantExecutionAdmission(err)
		return err
	}
	err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.activeCapacity.AcquireParticipantActiveSlot(txCtx, slot); err != nil {
			return err
		}
		if err := c.runs.Create(txCtx, run); err != nil {
			return err
		}
		return c.generations.Save(txCtx, generation, expectedVersion)
	})
	observeParticipantExecutionAdmission(err)
	if err == nil {
		observeParticipantExecutionStarted(generation.CreatedAt(), *startedAt)
	}
	return err
}

func (c *Committer) SaveDispatching(ctx context.Context, run *domainrun.AIExplanationRun) error {
	if run == nil || run.Status() != domainrun.StatusRunning || run.InvocationPhase() != domainrun.InvocationPhaseDispatching {
		return fmt.Errorf("dispatching AI explanation Run is required")
	}
	return c.runs.Save(ctx, run)
}

func (c *Committer) CommitSuccess(ctx context.Context, generation *domaingeneration.AIExplanationGeneration, run *domainrun.AIExplanationRun, artifact *domainartifact.AIExplanationArtifact, expectedVersion uint64) error {
	if err := validateSuccess(generation, run, artifact); err != nil {
		return err
	}
	generated, err := c.events.Generated(generation, run, artifact)
	if err != nil {
		return err
	}
	release, err := participantActiveSlotRelease(generation, run)
	if err != nil {
		return err
	}
	if err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.artifacts.Insert(txCtx, artifact); err != nil {
			return err
		}
		if err := c.runs.Save(txCtx, run); err != nil {
			return err
		}
		if err := c.generations.Save(txCtx, generation, expectedVersion); err != nil {
			return err
		}
		if err := c.stager.Stage(txCtx, generated); err != nil {
			return err
		}
		return c.activeCapacity.ReleaseParticipantActiveSlot(txCtx, release)
	}); err != nil {
		observeParticipantActiveSlotRelease(err)
		return err
	}
	observeParticipantActiveSlotRelease(nil)
	observeParticipantTerminal("generated", generation.CreatedAt(), *run.FinishedAt())
	c.postCommit.AfterCommit(ctx, []event.DomainEvent{generated}, generated.OccurredAt())
	return nil
}

func (c *Committer) CommitFailure(ctx context.Context, generation *domaingeneration.AIExplanationGeneration, run *domainrun.AIExplanationRun, expectedVersion uint64) error {
	if generation == nil || run == nil || generation.Status() != domaingeneration.StatusFailed || run.Status() != domainrun.StatusFailed || generation.LatestRunID() != run.ID() || run.GenerationID() != generation.ID() || run.Failure() == nil {
		return fmt.Errorf("failed AI explanation Generation and Run are required")
	}
	failed, err := c.events.Failed(generation, run)
	if err != nil {
		return err
	}
	release, err := participantActiveSlotRelease(generation, run)
	if err != nil {
		return err
	}
	if err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.runs.Save(txCtx, run); err != nil {
			return err
		}
		if err := c.generations.Save(txCtx, generation, expectedVersion); err != nil {
			return err
		}
		if err := c.stager.Stage(txCtx, failed); err != nil {
			return err
		}
		return c.activeCapacity.ReleaseParticipantActiveSlot(txCtx, release)
	}); err != nil {
		observeParticipantActiveSlotRelease(err)
		return err
	}
	observeParticipantActiveSlotRelease(nil)
	observeParticipantTerminal("failed", generation.CreatedAt(), *run.FinishedAt())
	c.postCommit.AfterCommit(ctx, []event.DomainEvent{failed}, failed.OccurredAt())
	return nil
}

func participantActiveSlotRelease(generation *domaingeneration.AIExplanationGeneration, run *domainrun.AIExplanationRun) (domaingeneration.ParticipantActiveSlotRelease, error) {
	if generation == nil || run == nil || run.FinishedAt() == nil || generation.LatestRunID() != run.ID() || run.GenerationID() != generation.ID() {
		return domaingeneration.ParticipantActiveSlotRelease{}, fmt.Errorf("terminal AI explanation active slot references are required")
	}
	association := generation.Association()
	return domaingeneration.ParticipantActiveSlotRelease{
		GenerationID: generation.ID(), RunID: run.ID(), OrgID: association.OrgID,
		UserID: generation.RequestedBy().ID, AssessmentID: association.AssessmentID, ReleasedAt: *run.FinishedAt(),
	}, nil
}

func validateSuccess(generation *domaingeneration.AIExplanationGeneration, run *domainrun.AIExplanationRun, artifact *domainartifact.AIExplanationArtifact) error {
	if generation == nil || run == nil || artifact == nil || generation.Status() != domaingeneration.StatusGenerated || run.Status() != domainrun.StatusSucceeded {
		return fmt.Errorf("generated AI explanation Generation, Run and Artifact are required")
	}
	if generation.LatestRunID() != run.ID() || generation.ArtifactID() != artifact.ID() || run.GenerationID() != generation.ID() || artifact.GenerationID() != generation.ID() || artifact.RunID() != run.ID() {
		return fmt.Errorf("AI explanation terminal references do not match")
	}
	return nil
}
