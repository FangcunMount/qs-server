package locklease

import (
	"time"

	base "github.com/FangcunMount/component-base/pkg/locklease"
)

type Spec = base.Spec
type Identity = base.Identity
type Lease = base.Lease
type Manager = base.Manager

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
