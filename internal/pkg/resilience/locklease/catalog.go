package locklease

import (
	"fmt"
	"strings"
	"time"
)

// WorkloadID is the stable identity of a built-in lock workload.
type WorkloadID string

const (
	WorkloadAnswersheetProcessing        WorkloadID = "answersheet_processing"
	WorkloadPlanSchedulerLeader          WorkloadID = "plan_scheduler_leader"
	WorkloadStatisticsSyncLeader         WorkloadID = "statistics_sync_leader"
	WorkloadStatisticsSync               WorkloadID = "statistics_sync"
	WorkloadEvaluationConsistencyAudit   WorkloadID = "evaluation_consistency_audit"
	WorkloadEvaluationLeaseRecovery      WorkloadID = "evaluation_lease_recovery"
	WorkloadInterpretationLeaseRecovery  WorkloadID = "interpretation_lease_recovery"
	WorkloadReportCatalogAudit           WorkloadID = "report_catalog_audit"
	WorkloadMongoConsistencyAudit        WorkloadID = "mongo_consistency_audit"
	WorkloadAttentionProjectionReconcile WorkloadID = "attention_projection_reconcile"
	WorkloadAuthzRoleProjectionReconcile WorkloadID = "authz_role_projection_reconcile"
	WorkloadCollectionSubmit             WorkloadID = "collection_submit"
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
	{WorkloadEvaluationConsistencyAudit, "apiserver", KindLeader, Spec{Name: string(WorkloadEvaluationConsistencyAudit), Description: "用于 apiserver Evaluation 一致性审计周期的单 leader 执行。", DefaultTTL: 30 * time.Second}, RenewalModeAuto},
	{WorkloadEvaluationLeaseRecovery, "apiserver", KindLeader, Spec{Name: string(WorkloadEvaluationLeaseRecovery), Description: "用于 apiserver Evaluation 过期运行租约恢复的单 leader 执行。", DefaultTTL: 30 * time.Second}, RenewalModeAuto},
	{WorkloadInterpretationLeaseRecovery, "apiserver", KindLeader, Spec{Name: string(WorkloadInterpretationLeaseRecovery), Description: "用于 apiserver Interpretation 过期运行租约恢复的单 leader 执行。", DefaultTTL: 30 * time.Second}, RenewalModeAuto},
	{WorkloadReportCatalogAudit, "apiserver", KindLeader, Spec{Name: string(WorkloadReportCatalogAudit), Description: "用于 apiserver 有界报告目录审计多实例 leader 选举与自动续租。", DefaultTTL: 30 * time.Second}, RenewalModeAuto},
	{WorkloadMongoConsistencyAudit, "apiserver", KindLeader, Spec{Name: string(WorkloadMongoConsistencyAudit), Description: "用于 apiserver Mongo 跨集合一致性只读巡检的单 leader 执行。", DefaultTTL: 30 * time.Second}, RenewalModeAuto},
	{WorkloadAttentionProjectionReconcile, "worker", KindLeader, Spec{Name: string(WorkloadAttentionProjectionReconcile), Description: "用于 worker Attention 失败重试与历史事实恢复的多实例 leader 选举。", DefaultTTL: 30 * time.Minute}, RenewalModeAuto},
	{WorkloadAuthzRoleProjectionReconcile, "apiserver", KindLeader, Spec{Name: string(WorkloadAuthzRoleProjectionReconcile), Description: "用于 apiserver 待同步 IAM 角色投影收敛的多实例 leader 选举。", DefaultTTL: 15 * time.Minute}, RenewalModeAuto},
	{WorkloadCollectionSubmit, "collection-server", KindDuplicateSuppression, Spec{Name: string(WorkloadCollectionSubmit), Description: "用于 collection-server 跨实例合并相同答卷提交的建议性 lease；最终幂等由 Mongo 裁决。", DefaultTTL: 5 * time.Minute}, RenewalModeAuto},
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
