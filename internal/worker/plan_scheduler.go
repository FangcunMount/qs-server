package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
	redis "github.com/redis/go-redis/v9"
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

	redisClient, err := s.dbManager.GetRedisClient()
	if err != nil {
		log.Warnf("worker plan scheduler not started (HA lock unavailable: %v)", err)
		return
	}

	runner := newWorkerPlanSchedulerRunner(opts, redisClient, s.grpcManager.PlanClient())
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
	redisClient redis.UniversalClient
	client      planSchedulerClient
	pingRedis   func(ctx context.Context) error
	acquireLock func(ctx context.Context, key string, ttl time.Duration) (string, bool, error)
	releaseLock func(ctx context.Context, key, token string) error
}

func newWorkerPlanSchedulerRunner(
	opts *workerconfig.PlanSchedulerConfig,
	redisClient redis.UniversalClient,
	client planSchedulerClient,
) *workerPlanSchedulerRunner {
	return newWorkerPlanSchedulerRunnerWithHooks(
		opts,
		redisClient,
		client,
		func(ctx context.Context) error {
			if redisClient == nil {
				return fmt.Errorf("redis client is nil")
			}
			return redisClient.Ping(ctx).Err()
		},
		func(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
			return redislock.Acquire(ctx, redisClient, key, ttl)
		},
		func(ctx context.Context, key, token string) error {
			return redislock.Release(ctx, redisClient, key, token)
		},
	)
}

func newWorkerPlanSchedulerRunnerWithHooks(
	opts *workerconfig.PlanSchedulerConfig,
	redisClient redis.UniversalClient,
	client planSchedulerClient,
	pingRedis func(ctx context.Context) error,
	acquireLock func(ctx context.Context, key string, ttl time.Duration) (string, bool, error),
	releaseLock func(ctx context.Context, key, token string) error,
) *workerPlanSchedulerRunner {
	if opts == nil || !opts.Enable {
		return nil
	}
	if client == nil {
		log.Warnf("worker plan scheduler not started (plan client unavailable)")
		return nil
	}
	if redisClient == nil {
		log.Warnf("worker plan scheduler not started (HA lock unavailable: redis client unavailable)")
		return nil
	}
	if pingRedis == nil {
		pingRedis = func(context.Context) error { return nil }
	}
	if err := pingRedis(context.Background()); err != nil {
		log.Warnf("worker plan scheduler not started (HA lock unavailable: %v)", err)
		return nil
	}
	if acquireLock == nil || releaseLock == nil {
		log.Warnf("worker plan scheduler not started (lock hooks unavailable)")
		return nil
	}

	return &workerPlanSchedulerRunner{
		opts:        opts,
		redisClient: redisClient,
		client:      client,
		pingRedis:   pingRedis,
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

	log.Infof("worker plan scheduler started (org_ids=%v, interval=%s, initial_delay=%s, lock_key=%s, lock_ttl=%s)",
		r.opts.OrgIDs, r.opts.Interval, r.opts.InitialDelay, r.opts.LockKey, r.opts.LockTTL)

	go func() {
		defer close(done)

		if !waitWorkerPlanSchedulerDelay(ctx, r.opts.InitialDelay) {
			return
		}

		r.executeTick(ctx)

		ticker := time.NewTicker(r.opts.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
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
	token, acquired, err := r.acquireLock(ctx, r.opts.LockKey, r.opts.LockTTL)
	if err != nil {
		return fmt.Errorf("failed to acquire worker plan scheduler lock: %w", err)
	}
	if !acquired {
		log.Infof("worker plan scheduler tick skipped (lock_key=%s, org_ids=%v, reason=lock_not_acquired)",
			r.opts.LockKey, r.opts.OrgIDs)
		return nil
	}

	defer func() {
		if err := r.releaseLock(context.Background(), r.opts.LockKey, token); err != nil {
			log.Warnf("failed to release worker plan scheduler lock (lock_key=%s): %v", r.opts.LockKey, err)
		}
	}()

	log.Infof("worker plan scheduler tick acquired lock (lock_key=%s, org_ids=%v)", r.opts.LockKey, r.opts.OrgIDs)

	totalOpened := 0
	totalExpired := 0
	failedOrgs := 0

	for _, orgID := range r.opts.OrgIDs {
		resp, err := r.client.SchedulePendingTasks(ctx, &pb.SchedulePendingTasksRequest{
			OrgId:  orgID,
			Before: "",
			Source: planApp.TaskSchedulerSourceBuiltin,
		})
		if err != nil {
			failedOrgs++
			log.Warnf("worker plan scheduler tick failed for org (org_id=%d, lock_key=%s): %v", orgID, r.opts.LockKey, err)
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
		r.opts.LockKey, r.opts.OrgIDs, totalOpened, totalExpired, failedOrgs)

	return nil
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
