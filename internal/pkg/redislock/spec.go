package redislock

import "time"

// Spec 描述一类分布式锁的统一规格。
type Spec struct {
	Name        string
	Description string
	DefaultTTL  time.Duration
}

// Identity 基于规格和业务键生成锁身份。
func (s Spec) Identity(key string) Identity {
	return Identity{
		Name: s.Name,
		Key:  key,
	}
}

// Specs 定义系统内置的分布式锁规格集合。
var Specs = struct {
	AnswersheetProcessing    Spec
	PlanSchedulerLeader      Spec
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
		Description: "用于 worker 计划调度器多实例抢占 leader 的分布式锁。",
		DefaultTTL:  50 * time.Second,
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
