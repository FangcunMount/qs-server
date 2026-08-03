package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

type planCommandService interface {
	SchedulePendingTasks(ctx context.Context, orgID int64, before string) (*planApp.TaskScheduleResult, error)
}

var planSchedulerBusinessLocation = func() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return location
}()

// PlanRunner executes built-in plan scheduling inside apiserver.
type PlanRunner struct {
	opts    *apiserveroptions.PlanSchedulerOptions
	command planCommandService
	leader  leaderLeaseRunner

	backlogMu          sync.Mutex
	missedBacklogTicks map[int64]int
}

// NewPlanRunner creates the apiserver plan scheduler runner.
func NewPlanRunner(
	opts *apiserveroptions.PlanSchedulerOptions,
	lockManager locklease.Manager,
	command planCommandService,
	lockBuilder *keyspace.Builder,
) *PlanRunner {
	return newPlanRunnerWithHooks(
		opts,
		lockManager,
		command,
		lockBuilder,
		func(ctx context.Context, spec locklease.Spec, key string, ttl time.Duration) (*locklease.Lease, bool, error) {
			return lockManager.AcquireSpec(ctx, spec, key, ttl)
		},
		func(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease) error {
			return lockManager.ReleaseSpec(ctx, spec, key, lease)
		},
	)
}

func newPlanRunnerWithHooks(
	opts *apiserveroptions.PlanSchedulerOptions,
	lockManager locklease.Manager,
	command planCommandService,
	lockBuilder *keyspace.Builder,
	acquireLock func(ctx context.Context, spec locklease.Spec, key string, ttl time.Duration) (*locklease.Lease, bool, error),
	releaseLock func(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease) error,
) *PlanRunner {
	if opts == nil || !opts.Enable {
		return nil
	}
	if command == nil {
		observability.ObserveLockDegraded("plan_scheduler_leader", "service_unavailable")
		log.Warnf("apiserver plan scheduler not started (plan command service unavailable)")
		return nil
	}
	if lockManager == nil {
		observability.ObserveLockDegraded("plan_scheduler_leader", "redis_unavailable")
		log.Warnf("apiserver plan scheduler not started (HA lock unavailable: redis client unavailable)")
		return nil
	}
	if acquireLock == nil || releaseLock == nil {
		log.Warnf("apiserver plan scheduler not started (lock hooks unavailable)")
		return nil
	}

	return &PlanRunner{
		opts:               opts,
		command:            command,
		missedBacklogTicks: make(map[int64]int),
		leader: newLeaderLock(
			workloadSpec(locklease.WorkloadPlanSchedulerLeader),
			opts.LockKey,
			opts.LockTTL,
			lockBuilder,
			acquireLock,
			releaseLock,
			leaseRunner(lockManager),
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
	log.Infof("apiserver plan scheduler started (org_ids=%v, interval=%s, initial_delay=%s, batch_size=%d, max_tasks_per_tick=%d, missed_expiration_enabled=%t, lock_key=%s, lock_ttl=%s)",
		r.opts.OrgIDs, r.opts.Interval, r.opts.InitialDelay, r.opts.BatchSize, r.opts.MaxTasksPerTick, r.opts.MissedExpirationEnabled, lockKey, r.opts.LockTTL)

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
			before := time.Now().In(planSchedulerBusinessLocation)
			lowerBound := before.Add(-planDomain.TaskOpenWindow)
			scheduleCtx := planApp.WithTaskSchedulerPlannedAtLowerBound(ctx, lowerBound)
			scheduleCtx = planApp.WithTaskSchedulerMissedExpirationEnabled(scheduleCtx, r.opts.MissedExpirationEnabled)
			scheduleCtx = planApp.WithTaskSchedulerScanLimits(scheduleCtx, r.opts.BatchSize, r.opts.MaxTasksPerTick)
			result, err := r.command.SchedulePendingTasks(scheduleCtx, orgID, before.Format("2006-01-02 15:04:05"))
			if result != nil {
				observePlanSchedulerStats(orgID, result.Stats)
				r.observeMissedBacklog(orgID, result.Stats.MissedBacklogCount)
			}
			if err != nil {
				failedOrgs++
				observePlanSchedulerOrganization("error")
				log.Warnf("apiserver plan scheduler tick failed for org (org_id=%d, lock_key=%s): %v", orgID, lockKey, err)
				continue
			}
			observePlanSchedulerOrganization("success")
			if result == nil {
				continue
			}
			totalOpened += result.Stats.OpenedCount
			totalExpired += result.Stats.ExpiredCount + result.Stats.MissedExpiredCount
		}

		log.Infof("apiserver plan scheduler tick completed (lock_key=%s, org_ids=%v, opened_count=%d, expired_count=%d, failed_org_count=%d)",
			lockKey, r.opts.OrgIDs, totalOpened, totalExpired, failedOrgs)

		if failedOrgs > 0 {
			return fmt.Errorf("plan scheduler failed for %d organization(s)", failedOrgs)
		}
		return nil
	})
}

func (r *PlanRunner) observeMissedBacklog(orgID int64, backlogCount int) {
	if r == nil {
		return
	}
	r.backlogMu.Lock()
	defer r.backlogMu.Unlock()
	if backlogCount <= 0 {
		delete(r.missedBacklogTicks, orgID)
		return
	}
	r.missedBacklogTicks[orgID]++
	if r.missedBacklogTicks[orgID] >= 2 {
		log.Warnf("apiserver plan scheduler missed backlog alert (org_id=%d, consecutive_ticks=%d, backlog_present=true)", orgID, r.missedBacklogTicks[orgID])
	}
}

func (r *PlanRunner) lockKey() string {
	if r == nil {
		return ""
	}
	return r.leader.DisplayKey()
}
