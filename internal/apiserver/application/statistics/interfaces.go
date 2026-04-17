package statistics

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

// ==================== 应用服务接口 ====================

// SystemStatisticsService 系统整体统计服务
type SystemStatisticsService interface {
	// GetSystemStatistics 获取系统整体统计
	GetSystemStatistics(ctx context.Context, orgID int64) (*statistics.SystemStatistics, error)
}

// QuestionnaireStatisticsService 问卷/量表统计服务
type QuestionnaireStatisticsService interface {
	// GetQuestionnaireStatistics 获取问卷/量表统计
	GetQuestionnaireStatistics(ctx context.Context, orgID int64, questionnaireCode string) (*statistics.QuestionnaireStatistics, error)
}

// TesteeStatisticsService 受试者统计服务
type TesteeStatisticsService interface {
	// GetTesteeStatistics 获取受试者统计
	GetTesteeStatistics(ctx context.Context, orgID int64, testeeID uint64) (*statistics.TesteeStatistics, error)
}

// PlanStatisticsService 测评计划统计服务
type PlanStatisticsService interface {
	// GetPlanStatistics 获取计划统计
	GetPlanStatistics(ctx context.Context, orgID int64, planID uint64) (*statistics.PlanStatistics, error)
}

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
	GetQuestionnaireBatchStatistics(ctx context.Context, orgID int64, codes []string) (*statistics.QuestionnaireBatchStatisticsResponse, error)
}

// PeriodicStatsService 受试者周期性统计服务。
type PeriodicStatsService interface {
	GetPeriodicStats(ctx context.Context, orgID int64, testeeID uint64) (*statistics.TesteePeriodicStatisticsResponse, error)
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
	// SyncAccumulatedStatistics 同步累计统计（MySQL 重建）
	SyncAccumulatedStatistics(ctx context.Context, orgID int64) error
	// SyncPlanStatistics 同步计划统计
	SyncPlanStatistics(ctx context.Context, orgID int64) error
}

// QueryFilter 通用统计查询过滤器。
type QueryFilter struct {
	Preset string
	From   string
	To     string
}
