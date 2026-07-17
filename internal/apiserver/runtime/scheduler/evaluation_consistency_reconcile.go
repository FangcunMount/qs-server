package scheduler

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	evaluationScheduler "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scheduler"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

// EvaluationConsistencyReconcileRunner
// 评估一致性协调器，定期修复评分/报告跨存储漂移。
type EvaluationConsistencyReconcileRunner struct {
	opts    *apiserveroptions.EvaluationConsistencyReconcileOptions
	service evaluationScheduler.Service
	leader  leaderLeaseRunner
}

// NewEvaluationConsistencyReconcileRunner 创建评估一致性协调器，当依赖项可用时创建协调器。
func NewEvaluationConsistencyReconcileRunner(
	opts *apiserveroptions.EvaluationConsistencyReconcileOptions,
	service evaluationScheduler.Service,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
) *EvaluationConsistencyReconcileRunner {
	return newEvaluationConsistencyReconcileRunnerWithHooks(
		opts,
		service,
		lockManager,
		lockBuilder,
		func(ctx context.Context, spec locklease.Spec, key string, ttl time.Duration) (*locklease.Lease, bool, error) {
			return lockManager.AcquireSpec(ctx, spec, key, ttl)
		},
		func(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease) error {
			return lockManager.ReleaseSpec(ctx, spec, key, lease)
		},
	)
}

// newEvaluationConsistencyReconcileRunnerWithHooks 创建评估一致性协调器，当依赖项可用时创建协调器。
// 参数：
// - opts: 配置选项
// - service: 评估一致性协调器服务
// - lockManager: 锁管理器
// - lockBuilder: 锁构建器
// - acquireLock: 获取锁函数
// - releaseLock: 释放锁函数
func newEvaluationConsistencyReconcileRunnerWithHooks(
	opts *apiserveroptions.EvaluationConsistencyReconcileOptions, // 配置选项
	service evaluationScheduler.Service, // 评估一致性协调器服务
	lockManager locklease.Manager, // 锁管理器
	lockBuilder *keyspace.Builder, // 锁构建器
	acquireLock func(ctx context.Context, spec locklease.Spec, key string, ttl time.Duration) (*locklease.Lease, bool, error), // 获取锁函数
	releaseLock func(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease) error, // 释放锁函数
) *EvaluationConsistencyReconcileRunner {
	if opts == nil || !opts.Enable { // 如果配置选项不可用或禁用，则返回 nil
		return nil
	}
	if service == nil {
		log.Warnf("evaluation consistency reconcile not started (service unavailable)")
		return nil
	}
	if opts.Interval <= 0 {
		log.Warnf("evaluation consistency reconcile not started (interval must be greater than 0)")
		return nil
	}
	if opts.BatchLimit <= 0 {
		log.Warnf("evaluation consistency reconcile not started (batch_limit must be greater than 0)")
		return nil
	}
	if opts.LockKey == "" {
		log.Warnf("evaluation consistency reconcile not started (lock_key is empty)")
		return nil
	}
	if opts.LockTTL <= 0 {
		log.Warnf("evaluation consistency reconcile not started (lock_ttl must be greater than 0)")
		return nil
	}
	if lockManager == nil {
		observability.ObserveLockDegraded("evaluation_consistency_reconcile", "redis_unavailable")
		log.Warnf("evaluation consistency reconcile not started (HA lock unavailable: redis client unavailable)")
		return nil
	}
	if acquireLock == nil || releaseLock == nil {
		log.Warnf("evaluation consistency reconcile not started (lock hooks unavailable)")
		return nil
	}

	// 创建评估一致性协调器
	return &EvaluationConsistencyReconcileRunner{
		opts:    opts,                                                                                                                                                                       // 配置选项
		service: service,                                                                                                                                                                    // 评估一致性协调器服务
		leader:  newLeaderLock(workloadSpec(locklease.WorkloadEvaluationConsistencyReconcile), opts.LockKey, opts.LockTTL, lockBuilder, acquireLock, releaseLock, leaseRunner(lockManager)), // 领导者锁
	}
}

// Name 返回协调器名称。
func (r *EvaluationConsistencyReconcileRunner) Name() string {
	return "evaluation_consistency_reconcile"
}

// Start 启动协调器循环。
func (r *EvaluationConsistencyReconcileRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}

	lockKey := r.lockKey() // 获取锁键
	log.Infof("evaluation consistency reconcile started (interval=%s, batch_limit=%d, lock_key=%s, lock_ttl=%s)",
		r.opts.Interval, r.opts.BatchLimit, lockKey, r.opts.LockTTL)

	go func() {
		// 执行第一次扫描
		r.executeTick(ctx)
		// 创建定时器，每隔一段时间执行一次扫描
		ticker := time.NewTicker(r.opts.Interval)
		defer ticker.Stop()
		// 循环执行扫描
		for {
			select {
			case <-ctx.Done(): // 上下文取消，退出循环
				return
			case <-ticker.C: // 定时器触发，执行扫描
				r.executeTick(ctx)
			}
		}
	}()
}

// executeTick 执行一次扫描。
func (r *EvaluationConsistencyReconcileRunner) executeTick(ctx context.Context) {
	// 执行一次扫描
	if err := r.runOnce(ctx); err != nil {
		log.Warnf("evaluation consistency reconcile failed: %v", err)
	}
}

// runOnce 执行一次扫描。
func (r *EvaluationConsistencyReconcileRunner) runOnce(ctx context.Context) error {
	// 执行一次扫描
	return r.leader.Run(ctx, leaderLockRunOptions{
		AcquireError: "failed to acquire evaluation consistency reconcile lock",
		OnNotAcquired: func(lockKey string) {
			log.Debugf("evaluation consistency reconcile tick skipped (lock_key=%s, reason=lock_not_acquired)", lockKey)
		},
		OnReleaseError: func(lockKey string, err error) {
			log.Warnf("failed to release evaluation consistency reconcile lock (lock_key=%s): %v", lockKey, err)
		},
	}, func(ctx context.Context) error {
		_, err := r.service.AuditOnce(ctx, r.opts.BatchLimit)
		return err
	})
}

// lockKey 返回锁键。
func (r *EvaluationConsistencyReconcileRunner) lockKey() string {
	if r == nil {
		return ""
	}
	return r.leader.DisplayKey()
}
