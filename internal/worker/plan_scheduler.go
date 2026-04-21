package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
)

type planSchedulerClient interface {
	SchedulePendingTasks(ctx context.Context, req *pb.SchedulePendingTasksRequest) (*pb.SchedulePendingTasksResponse, error)
}

func (s *workerServer) startPlanScheduler() {
	if s == nil || s.config == nil {
		return
	}

	opts := s.config.PlanScheduler
	if opts == nil || !opts.Enable {
		return
	}

	if s.grpcManager == nil || s.grpcManager.PlanClient() == nil {
		log.Warnf("worker plan scheduler not started (plan gRPC client unavailable)")
		return
	}

	runner := newWorkerPlanSchedulerRunner(opts, s.lockManager, s.grpcManager.PlanClient(), s.lockKeyBuilder())
	if runner == nil {
		log.Warnf("worker plan scheduler disabled at runtime because required dependencies are unavailable")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		cancel()
		return nil
	}))
	runner.start(ctx)
}

type workerPlanSchedulerRunner struct {
	opts        *workerconfig.PlanSchedulerConfig
	lockManager *redislock.Manager
	client      planSchedulerClient
	lockBuilder *rediskey.Builder
	acquireLock func(ctx context.Context, spec redislock.Spec, key string, ttl time.Duration) (*redislock.Lease, bool, error)
	releaseLock func(ctx context.Context, spec redislock.Spec, key string, lease *redislock.Lease) error
}

func newWorkerPlanSchedulerRunner(
	opts *workerconfig.PlanSchedulerConfig,
	lockManager *redislock.Manager,
	client planSchedulerClient,
	lockBuilder *rediskey.Builder,
) *workerPlanSchedulerRunner {
	return newWorkerPlanSchedulerRunnerWithHooks(
		opts,
		lockManager,
		client,
		lockBuilder,
		func(ctx context.Context, spec redislock.Spec, key string, ttl time.Duration) (*redislock.Lease, bool, error) {
			return lockManager.AcquireSpec(ctx, spec, key, ttl)
		},
		func(ctx context.Context, spec redislock.Spec, key string, lease *redislock.Lease) error {
			return lockManager.ReleaseSpec(ctx, spec, key, lease)
		},
	)
}

func newWorkerPlanSchedulerRunnerWithHooks(
	opts *workerconfig.PlanSchedulerConfig,
	lockManager *redislock.Manager,
	client planSchedulerClient,
	lockBuilder *rediskey.Builder,
	acquireLock func(ctx context.Context, spec redislock.Spec, key string, ttl time.Duration) (*redislock.Lease, bool, error),
	releaseLock func(ctx context.Context, spec redislock.Spec, key string, lease *redislock.Lease) error,
) *workerPlanSchedulerRunner {
	if opts == nil || !opts.Enable {
		return nil
	}
	if client == nil {
		cacheobservability.ObserveLockDegraded("plan_scheduler_leader", "client_unavailable")
		log.Warnf("worker plan scheduler not started (plan client unavailable)")
		return nil
	}
	if lockManager == nil {
		cacheobservability.ObserveLockDegraded("plan_scheduler_leader", "redis_unavailable")
		log.Warnf("worker plan scheduler not started (HA lock unavailable: redis client unavailable)")
		return nil
	}
	if acquireLock == nil || releaseLock == nil {
		log.Warnf("worker plan scheduler not started (lock hooks unavailable)")
		return nil
	}

	return &workerPlanSchedulerRunner{
		opts:        opts,
		lockManager: lockManager,
		client:      client,
		lockBuilder: lockBuilder,
		acquireLock: acquireLock,
		releaseLock: releaseLock,
	}
}

func (r *workerPlanSchedulerRunner) start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	if r == nil {
		close(done)
		return done
	}

	lockKey := r.lockKey()
	log.Infof("worker plan scheduler started (org_ids=%v, interval=%s, initial_delay=%s, lock_key=%s, lock_ttl=%s)",
		r.opts.OrgIDs, r.opts.Interval, r.opts.InitialDelay, lockKey, r.opts.LockTTL)

	go func() {
		defer close(done)

		if !waitWorkerPlanSchedulerDelay(ctx, r.opts.InitialDelay) {
			return
		}

		r.executeTick(ctx)

		for {
			if !waitWorkerPlanSchedulerUntilNextTick(ctx, r.opts.Interval) {
				return
			}
			r.executeTick(ctx)
		}
	}()

	return done
}

func (r *workerPlanSchedulerRunner) executeTick(ctx context.Context) {
	if err := r.runOnce(ctx); err != nil {
		log.Warnf("worker plan scheduler tick failed: %v", err)
	}
}

func (r *workerPlanSchedulerRunner) runOnce(ctx context.Context) error {
	lockKey := r.lockKey()
	lockSpec := redislock.Specs.PlanSchedulerLeader

	lease, acquired, err := r.acquireLock(ctx, lockSpec, r.opts.LockKey, r.opts.LockTTL)
	if err != nil {
		return fmt.Errorf("failed to acquire worker plan scheduler lock: %w", err)
	}
	if !acquired {
		log.Infof("worker plan scheduler tick skipped (lock_key=%s, org_ids=%v, reason=lock_not_acquired)",
			lockKey, r.opts.OrgIDs)
		return nil
	}

	defer func() {
		if err := r.releaseLock(context.Background(), lockSpec, r.opts.LockKey, lease); err != nil {
			log.Warnf("failed to release worker plan scheduler lock (lock_key=%s): %v", lockKey, err)
		}
	}()

	log.Infof("worker plan scheduler tick acquired lock (lock_key=%s, org_ids=%v)", lockKey, r.opts.OrgIDs)

	totalOpened := 0
	totalExpired := 0
	failedOrgs := 0

	for _, orgID := range r.opts.OrgIDs {
		resp, err := r.client.SchedulePendingTasks(ctx, &pb.SchedulePendingTasksRequest{
			OrgId:  orgID,
			Before: "",
		})
		if err != nil {
			failedOrgs++
			log.Warnf("worker plan scheduler tick failed for org (org_id=%d, lock_key=%s): %v", orgID, lockKey, err)
			continue
		}

		if resp.GetStats() != nil {
			totalOpened += int(resp.GetStats().GetOpenedCount())
			totalExpired += int(resp.GetStats().GetExpiredCount())
			continue
		}
		totalOpened += len(resp.GetTasks())
	}

	log.Infof("worker plan scheduler tick completed (lock_key=%s, org_ids=%v, opened_count=%d, expired_count=%d, failed_org_count=%d)",
		lockKey, r.opts.OrgIDs, totalOpened, totalExpired, failedOrgs)

	return nil
}

func (r *workerPlanSchedulerRunner) lockKey() string {
	if r == nil {
		return ""
	}
	if r.lockBuilder == nil {
		r.lockBuilder = rediskey.NewBuilder()
	}
	return r.lockBuilder.BuildLockKey(r.opts.LockKey)
}

func waitWorkerPlanSchedulerDelay(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func waitWorkerPlanSchedulerUntilNextTick(ctx context.Context, interval time.Duration) bool {
	nextTickAt := nextWorkerPlanSchedulerTickTime(time.Now(), interval)
	return waitWorkerPlanSchedulerDelay(ctx, time.Until(nextTickAt))
}

func nextWorkerPlanSchedulerTickTime(now time.Time, interval time.Duration) time.Time {
	if interval <= 0 {
		return now
	}
	if interval%time.Minute != 0 {
		return now.Add(interval)
	}

	loc := now.Location()
	if loc == nil {
		loc = time.Local
	}

	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	currentMinute := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, loc)
	nextOffset := (currentMinute.Sub(midnight)/interval + 1) * interval
	return midnight.Add(nextOffset)
}
