package locklease

import (
	"fmt"
	"strings"
	"time"
)

// WorkloadID is the stable identity of a built-in lock workload.
type WorkloadID string

const (
	WorkloadAnswersheetProcessing          WorkloadID = "answersheet_processing"
	WorkloadPlanSchedulerLeader            WorkloadID = "plan_scheduler_leader"
	WorkloadStatisticsSyncLeader           WorkloadID = "statistics_sync_leader"
	WorkloadStatisticsSync                 WorkloadID = "statistics_sync"
	WorkloadBehaviorPendingReconcile       WorkloadID = "behavior_pending_reconcile"
	WorkloadEvaluationConsistencyReconcile WorkloadID = "evaluation_consistency_reconcile"
	WorkloadBehaviorJourneyScanLeader      WorkloadID = "behavior_journey_scan_leader"
	WorkloadCollectionSubmit               WorkloadID = "collection_submit"
)

// Kind classifies the business semantics of a lease workload.
type Kind string

const (
	KindLeader               Kind = "leader"
	KindTaskLock             Kind = "task_lock"
	KindIdempotency          Kind = "idempotency"
	KindDuplicateSuppression Kind = "duplicate_suppression"
)

// RenewalMode describes the immutable renewal policy of a workload.
type RenewalMode string

const (
	RenewalModeAuto RenewalMode = "auto"
)

// Capability is one immutable catalog entry.
type Capability struct {
	ID          WorkloadID
	Component   string
	Kind        Kind
	Spec        Spec
	RenewalMode RenewalMode
}

var capabilities = [...]Capability{
	{WorkloadAnswersheetProcessing, "worker", KindDuplicateSuppression, Spec{Name: string(WorkloadAnswersheetProcessing), Description: "用于抑制同一答卷提交事件被重复处理的 best-effort 分布式锁。", DefaultTTL: 5 * time.Minute}, RenewalModeAuto},
	{WorkloadPlanSchedulerLeader, "apiserver", KindLeader, Spec{Name: string(WorkloadPlanSchedulerLeader), Description: "用于 apiserver 计划调度器多实例抢占 leader 的分布式锁。", DefaultTTL: 50 * time.Second}, RenewalModeAuto},
	{WorkloadStatisticsSyncLeader, "apiserver", KindLeader, Spec{Name: string(WorkloadStatisticsSyncLeader), Description: "用于 apiserver 统计同步调度器多实例抢占 leader 的分布式锁。", DefaultTTL: 30 * time.Minute}, RenewalModeAuto},
	{WorkloadStatisticsSync, "apiserver", KindTaskLock, Spec{Name: string(WorkloadStatisticsSync), Description: "用于 apiserver 统计同步任务串行化执行的分布式锁。", DefaultTTL: 30 * time.Minute}, RenewalModeAuto},
	{WorkloadBehaviorPendingReconcile, "apiserver", KindLeader, Spec{Name: string(WorkloadBehaviorPendingReconcile), Description: "用于 apiserver behavior pending reconcile 多实例串行化执行的分布式锁。", DefaultTTL: 30 * time.Second}, RenewalModeAuto},
	{WorkloadEvaluationConsistencyReconcile, "apiserver", KindLeader, Spec{Name: string(WorkloadEvaluationConsistencyReconcile), Description: "用于 apiserver evaluation consistency reconcile 多实例串行化执行的分布式锁。", DefaultTTL: 30 * time.Second}, RenewalModeAuto},
	{WorkloadBehaviorJourneyScanLeader, "apiserver", KindLeader, Spec{Name: string(WorkloadBehaviorJourneyScanLeader), Description: "用于 apiserver behavior journey scan 多实例抢占 leader 的分布式锁。", DefaultTTL: 25 * time.Minute}, RenewalModeAuto},
	{WorkloadCollectionSubmit, "collection-server", KindIdempotency, Spec{Name: string(WorkloadCollectionSubmit), Description: "用于 collection-server 答卷提交幂等与进行中抑制的分布式锁。", DefaultTTL: 5 * time.Minute}, RenewalModeAuto},
}

// Lookup returns a copy of one catalog entry.
func Lookup(id WorkloadID) (Capability, bool) {
	for _, capability := range capabilities {
		if capability.ID == id {
			return capability, true
		}
	}
	return Capability{}, false
}

// All returns a copy of the immutable built-in catalog.
func All() []Capability {
	result := make([]Capability, len(capabilities))
	copy(result, capabilities[:])
	return result
}

// ValidateCatalog verifies all invariants required by configuration and governance projections.
func ValidateCatalog() error {
	seenIDs := make(map[WorkloadID]struct{}, len(capabilities))
	seenNames := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if capability.ID == "" || strings.TrimSpace(capability.Component) == "" {
			return fmt.Errorf("lock capability id/component is empty")
		}
		if _, exists := seenIDs[capability.ID]; exists {
			return fmt.Errorf("duplicate lock capability id %q", capability.ID)
		}
		seenIDs[capability.ID] = struct{}{}
		if capability.Spec.Name == "" || capability.Spec.Name != string(capability.ID) {
			return fmt.Errorf("lock capability %q has invalid spec name %q", capability.ID, capability.Spec.Name)
		}
		if _, exists := seenNames[capability.Spec.Name]; exists {
			return fmt.Errorf("duplicate lock capability name %q", capability.Spec.Name)
		}
		seenNames[capability.Spec.Name] = struct{}{}
		if strings.TrimSpace(capability.Spec.Description) == "" || capability.Spec.DefaultTTL <= 0 {
			return fmt.Errorf("lock capability %q has invalid spec", capability.ID)
		}
		switch capability.Kind {
		case KindLeader, KindTaskLock, KindIdempotency, KindDuplicateSuppression:
		default:
			return fmt.Errorf("lock capability %q has invalid kind %q", capability.ID, capability.Kind)
		}
		if capability.RenewalMode != RenewalModeAuto {
			return fmt.Errorf("lock capability %q has invalid renewal mode %q", capability.ID, capability.RenewalMode)
		}
	}
	return nil
}
