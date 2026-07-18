package statistics

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

// ==================== 应用服务接口 ====================

// ReadService v1 统一统计读服务。
type ReadService interface {
	GetOverview(ctx context.Context, orgID int64, filter QueryFilter) (*statistics.StatisticsOverview, error)
	ListClinicianStatistics(ctx context.Context, orgID int64, filter QueryFilter, page, pageSize int) (*statistics.ClinicianStatisticsList, error)
	GetClinicianStatistics(ctx context.Context, orgID int64, clinicianID uint64, filter QueryFilter) (*statistics.ClinicianStatistics, error)
	ListAssessmentEntryStatistics(ctx context.Context, orgID int64, clinicianID *uint64, activeOnly *bool, filter QueryFilter, page, pageSize int) (*statistics.AssessmentEntryStatisticsList, error)
	GetAssessmentEntryStatistics(ctx context.Context, orgID int64, entryID uint64, filter QueryFilter) (*statistics.AssessmentEntryStatistics, error)
	GetCurrentClinicianStatistics(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter) (*statistics.ClinicianStatistics, error)
	ListCurrentClinicianEntryStatistics(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter, page, pageSize int) (*statistics.AssessmentEntryStatisticsList, error)
	GetCurrentClinicianTesteeSummary(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter) (*statistics.ClinicianTesteeSummaryStatistics, error)
	GetContentBatchStatistics(ctx context.Context, orgID int64, refs []statistics.ContentReference, access ContentStatisticsAccess) (*statistics.ContentBatchStatisticsResponse, error)
}

// ContentStatisticsAccess describes which typed content families the caller may query.
type ContentStatisticsAccess struct {
	Questionnaire bool
	Scale         bool
}

// PeriodicStatsService 受试者周期性统计服务。
type PeriodicStatsService interface {
	GetPeriodicStats(ctx context.Context, orgID int64, testeeID uint64) (*statistics.TesteePeriodicStatisticsResponse, error)
}

// GovernanceFacade 统计治理入口 门面。
type GovernanceFacade interface {
	TriggerStatisticsWarmup(ctx context.Context, orgID int64, action string)
	HandleRepairComplete(ctx context.Context, protectedOrgID int64, req RepairCompleteRequest) error
	HandleManualWarmup(ctx context.Context, protectedOrgID int64, req ManualWarmupRequest) (*cachemodel.ManualWarmupResult, error)
	GetStatus(ctx context.Context) (*cachemodel.StatusSnapshot, error)
	GetHotset(ctx context.Context, kindRaw, limitRaw string) (*GovernanceHotsetResponse, error)
}

// WarmupCoordinator is the application-owned port consumed by statistics.
type WarmupCoordinator interface {
	WarmStartup(context.Context) error
	HandleScalePublished(context.Context, string) error
	HandleQuestionnairePublished(context.Context, string, string) error
	HandleTypologyModelPublished(context.Context, string) error
	HandleStatisticsSync(context.Context, int64) error
	HandleRepairComplete(context.Context, cachetarget.RepairCompleteRequest) error
	HandleManualWarmup(context.Context, cachetarget.ManualWarmupRequest) (*cachemodel.ManualWarmupResult, error)
}

// GovernanceStatusReader is the application-owned cache status port.
type GovernanceStatusReader interface {
	GetRuntime(context.Context) (*cachemodel.RuntimeSnapshot, error)
	GetStatus(context.Context) (*cachemodel.StatusSnapshot, error)
	GetHotset(context.Context, cachetarget.WarmupKind, int64) (*cachetarget.HotsetSnapshot, error)
}

// RepairCompleteRequest 描述 repair complete 治理请求。
type RepairCompleteRequest struct {
	RepairKind string  `json:"repair_kind"`
	OrgIDs     []int64 `json:"org_ids"`
}

// ManualWarmupRequest 描述手工治理预热请求。
type ManualWarmupRequest = cachetarget.ManualWarmupRequest

// GovernanceHotsetResponse 描述治理热集响应。
type GovernanceHotsetResponse struct {
	Family    cachemodel.Family        `json:"family,omitempty"`
	Kind      cachetarget.WarmupKind   `json:"kind,omitempty"`
	Limit     int64                    `json:"limit,omitempty"`
	Available bool                     `json:"available"`
	Degraded  bool                     `json:"degraded"`
	Message   string                   `json:"message,omitempty"`
	Items     []cachetarget.HotsetItem `json:"items"`
}

// SyncDailyOptions 每日统计同步窗口。
// StartDate/EndDate 使用本地时区日界线，EndDate 为开区间。
type SyncDailyOptions struct {
	StartDate *time.Time
	EndDate   *time.Time
}

// StatisticsSyncService 统计同步服务（定时任务）
type StatisticsSyncService interface {
	// SyncDailyStatistics 同步每日统计（原始表 → MySQL）
	SyncDailyStatistics(ctx context.Context, orgID int64, opts SyncDailyOptions) error
	// SyncOrgSnapshotStatistics 同步机构总览快照（原始表 → statistics_org_snapshot）
	SyncOrgSnapshotStatistics(ctx context.Context, orgID int64) error
	// SyncPlanStatistics 同步计划统计
	SyncPlanStatistics(ctx context.Context, orgID int64) error
}

// QueryFilter 通用统计查询过滤器。
type QueryFilter struct {
	Preset string
	From   string
	To     string
}
