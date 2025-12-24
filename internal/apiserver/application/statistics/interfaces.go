package statistics

import (
	"context"

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

// ScreeningStatisticsService 筛查项目统计服务
type ScreeningStatisticsService interface {
	// GetScreeningStatistics 获取筛查项目统计
	GetScreeningStatistics(ctx context.Context, orgID int64, screeningID uint64) (*statistics.ScreeningStatistics, error)
}

// StatisticsSyncService 统计同步服务（定时任务）
type StatisticsSyncService interface {
	// SyncDailyStatistics 同步每日统计（Redis → MySQL）
	SyncDailyStatistics(ctx context.Context) error
	// SyncAccumulatedStatistics 同步累计统计（Redis → MySQL）
	SyncAccumulatedStatistics(ctx context.Context) error
	// SyncPlanStatistics 同步计划统计
	SyncPlanStatistics(ctx context.Context) error
}

// StatisticsValidatorService 统计校验服务（定时任务）
type StatisticsValidatorService interface {
	// ValidateConsistency 校验数据一致性（Redis vs MySQL）
	ValidateConsistency(ctx context.Context) error
}
