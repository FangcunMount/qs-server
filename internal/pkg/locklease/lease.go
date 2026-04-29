package locklease

import (
	"context"
	"time"
)

// Spec describes one semantic distributed lease-lock workload.
type Spec struct {
	Name        string
	Description string
	DefaultTTL  time.Duration
}

// Identity builds the concrete lease identity for a business key.
func (s Spec) Identity(key string) Identity {
	return Identity{
		Name: s.Name,
		Key:  key,
	}
}

// Identity describes one concrete lock instance.
type Identity struct {
	Name string
	Key  string
}

// Lease represents a successfully acquired lock lease.
type Lease struct {
	Key   string
	Token string
}

// Manager is the Redis-free lease-lock port used by business code.
type Manager interface {
	AcquireSpec(ctx context.Context, spec Spec, key string, ttlOverride ...time.Duration) (*Lease, bool, error)
	ReleaseSpec(ctx context.Context, spec Spec, key string, lease *Lease) error
}

// Specs defines qs-server built-in lease-lock workloads.
var Specs = struct {
	AnswersheetProcessing    Spec
	PlanSchedulerLeader      Spec
	StatisticsSyncLeader     Spec
	StatisticsSync           Spec
	BehaviorPendingReconcile Spec
	CollectionSubmit         Spec
}{
	AnswersheetProcessing: Spec{
		Name:        "answersheet_processing",
		Description: "用于抑制同一答卷提交事件被重复处理的 best-effort 分布式锁。",
		DefaultTTL:  5 * time.Minute,
	},
	PlanSchedulerLeader: Spec{
		Name:        "plan_scheduler_leader",
		Description: "用于 apiserver 计划调度器多实例抢占 leader 的分布式锁。",
		DefaultTTL:  50 * time.Second,
	},
	StatisticsSyncLeader: Spec{
		Name:        "statistics_sync_leader",
		Description: "用于 apiserver 统计同步调度器多实例抢占 leader 的分布式锁。",
		DefaultTTL:  30 * time.Minute,
	},
	StatisticsSync: Spec{
		Name:        "statistics_sync",
		Description: "用于 apiserver 统计同步任务串行化执行的分布式锁。",
		DefaultTTL:  30 * time.Minute,
	},
	BehaviorPendingReconcile: Spec{
		Name:        "behavior_pending_reconcile",
		Description: "用于 apiserver behavior pending reconcile 多实例串行化执行的分布式锁。",
		DefaultTTL:  30 * time.Second,
	},
	CollectionSubmit: Spec{
		Name:        "collection_submit",
		Description: "用于 collection-server 答卷提交幂等与进行中抑制的分布式锁。",
		DefaultTTL:  5 * time.Minute,
	},
}
