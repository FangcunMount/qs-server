package scheduler

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
)

type planCommandService interface {
	SchedulePendingTasks(ctx context.Context, orgID int64, before string) (*planApp.TaskScheduleResult, error)
}

// PlanRunner executes built-in plan scheduling inside apiserver.
type PlanRunner struct {
	opts    *apiserveroptions.PlanSchedulerOptions
	command planCommandService
	leader  *leaderLock
}

// NewPlanRunner creates the apiserver plan scheduler runner.
func NewPlanRunner(
	opts *apiserveroptions.PlanSchedulerOptions,
	lockManager *redislock.Manager,
	command planCommandService,
	lockBuilder *rediskey.Builder,
) *PlanRunner {
	return newPlanRunnerWithHooks(
		opts,
		lockManager,
		command,
		lockBuilder,
		func(ctx context.Context, spec redislock.Spec, key string, ttl time.Duration) (*redislock.Lease, bool, error) {
			return lockManager.AcquireSpec(ctx, spec, key, ttl)
		},
		func(ctx context.Context, spec redislock.Spec, key string, lease *redislock.Lease) error {
			return lockManager.ReleaseSpec(ctx, spec, key, lease)
		},
	)
}

func newPlanRunnerWithHooks(
	opts *apiserveroptions.PlanSchedulerOptions,
	lockManager *redislock.Manager,
	command planCommandService,
	lockBuilder *rediskey.Builder,
	acquireLock func(ctx context.Context, spec redislock.Spec, key string, ttl time.Duration) (*redislock.Lease, bool, error),
	releaseLock func(ctx context.Context, spec redislock.Spec, key string, lease *redislock.Lease) error,
) *PlanRunner {
	if opts == nil || !opts.Enable {
		return nil
	}
	if command == nil {
		cacheobservability.ObserveLockDegraded("plan_scheduler_leader", "service_unavailable")
		log.Warnf("apiserver plan scheduler not started (plan command service unavailable)")
		return nil
	}
	if lockManager == nil {
		cacheobservability.ObserveLockDegraded("plan_scheduler_leader", "redis_unavailable")
		log.Warnf("apiserver plan scheduler not started (HA lock unavailable: redis client unavailable)")
		return nil
	}
	if acquireLock == nil || releaseLock == nil {
		log.Warnf("apiserver plan scheduler not started (lock hooks unavailable)")
		return nil
	}

	return &PlanRunner{
		opts:    opts,
		command: command,
		leader: newLeaderLock(
			redislock.Specs.PlanSchedulerLeader,
			opts.LockKey,
			opts.LockTTL,
			lockBuilder,
			acquireLock,
			releaseLock,
		),
	}
}

// Name returns the runner name.
func (r *PlanRunner) Name() string {
	return "plan_scheduler"
}

// Start starts the plan scheduler loop.
func (r *PlanRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}

	lockKey := r.lockKey()
	log.Infof("apiserver plan scheduler started (org_ids=%v, interval=%s, initial_delay=%s, lock_key=%s, lock_ttl=%s)",
		r.opts.OrgIDs, r.opts.Interval, r.opts.InitialDelay, lockKey, r.opts.LockTTL)

	go func() {
		if !WaitDelay(ctx, r.opts.InitialDelay) {
			return
		}

		r.executeTick(ctx)

		for {
			if !WaitUntilNextAlignedInterval(ctx, r.opts.Interval) {
				return
			}
			r.executeTick(ctx)
		}
	}()
}

func (r *PlanRunner) executeTick(ctx context.Context) {
	if err := r.runOnce(ctx); err != nil {
		log.Warnf("apiserver plan scheduler tick failed: %v", err)
	}
}

func (r *PlanRunner) runOnce(ctx context.Context) error {
	lockKey := r.lockKey()

	return r.leader.Run(ctx, leaderLockRunOptions{
		AcquireError: "failed to acquire apiserver plan scheduler lock",
		OnNotAcquired: func(lockKey string) {
			log.Infof("apiserver plan scheduler tick skipped (lock_key=%s, org_ids=%v, reason=lock_not_acquired)",
				lockKey, r.opts.OrgIDs)
		},
		OnReleaseError: func(lockKey string, err error) {
			log.Warnf("failed to release apiserver plan scheduler lock (lock_key=%s): %v", lockKey, err)
		},
	}, func(ctx context.Context) error {
		log.Infof("apiserver plan scheduler tick acquired lock (lock_key=%s, org_ids=%v)", lockKey, r.opts.OrgIDs)

		totalOpened := 0
		totalExpired := 0
		failedOrgs := 0

		for _, orgID := range r.opts.OrgIDs {
			result, err := r.command.SchedulePendingTasks(ctx, orgID, "")
			if err != nil {
				failedOrgs++
				log.Warnf("apiserver plan scheduler tick failed for org (org_id=%d, lock_key=%s): %v", orgID, lockKey, err)
				continue
			}
			if result == nil {
				continue
			}
			totalOpened += result.Stats.OpenedCount
			totalExpired += result.Stats.ExpiredCount
		}

		log.Infof("apiserver plan scheduler tick completed (lock_key=%s, org_ids=%v, opened_count=%d, expired_count=%d, failed_org_count=%d)",
			lockKey, r.opts.OrgIDs, totalOpened, totalExpired, failedOrgs)

		return nil
	})
}

func (r *PlanRunner) lockKey() string {
	if r == nil {
		return ""
	}
	return r.leader.DisplayKey()
}
