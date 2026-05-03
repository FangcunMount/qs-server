package assembler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsReadModelInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics/readmodel"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	redis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// StatisticsModule 统计模块
type StatisticsModule struct {
	// service 层
	SystemStatisticsService        statisticsApp.SystemStatisticsService
	QuestionnaireStatisticsService statisticsApp.QuestionnaireStatisticsService
	TesteeStatisticsService        statisticsApp.TesteeStatisticsService
	PlanStatisticsService          statisticsApp.PlanStatisticsService
	ReadService                    statisticsApp.ReadService
	PeriodicStatsService           statisticsApp.PeriodicStatsService
	SyncService                    statisticsApp.StatisticsSyncService
	BehaviorProjectorService       statisticsApp.BehaviorProjectorService
}

// StatisticsModuleDeps 定义 Statistics 模块的显式构造依赖。
type StatisticsModuleDeps struct {
	MySQLDB           *gorm.DB
	RedisClient       redis.UniversalClient
	CacheBuilder      *keyspace.Builder
	AnswerSheetReader surveyreadmodel.AnswerSheetReader
	RepairWindowDays  int
	QueryPolicy       cachepolicy.CachePolicy
	HotsetRecorder    cachetarget.HotsetRecorder
	LockManager       locklease.Manager
	VersionStore      cachequery.VersionTokenStore
	Observer          *observability.ComponentObserver
	MySQLLimiter      backpressure.Acquirer
	WarmupCoordinator cachegov.Coordinator
	StatusService     cachegov.StatusService
}

// NewStatisticsModule 创建统计模块。
func NewStatisticsModule(deps StatisticsModuleDeps) (*StatisticsModule, error) {
	normalized, err := normalizeStatisticsModuleDeps(deps)
	if err != nil {
		return nil, err
	}
	module := &StatisticsModule{}

	// 初始化 repository 层
	repo := statisticsInfra.NewStatisticsRepository(normalized.MySQLDB, mysql.BaseRepositoryOptions{
		Limiter: normalized.MySQLLimiter,
	})

	// 初始化 cache 层
	var cache *statisticsCache.StatisticsCache
	if normalized.RedisClient != nil {
		cache = statisticsCache.NewStatisticsCacheWithBuilderPolicyVersionStoreAndObserver(
			normalized.RedisClient,
			normalized.CacheBuilder,
			normalized.QueryPolicy,
			normalized.VersionStore,
			normalized.Observer,
		)
	} else {
		// Redis不可用时，创建空实现（查询时会降级到MySQL）
		cache = nil
	}
	txRunner := newMySQLTransactionRunner(normalized.MySQLDB)

	// 初始化 service 层
	module.SystemStatisticsService = statisticsApp.NewSystemStatisticsService(repo, repo, cache, normalized.HotsetRecorder)
	module.QuestionnaireStatisticsService = statisticsApp.NewQuestionnaireStatisticsService(repo, repo, cache, normalized.HotsetRecorder)
	module.TesteeStatisticsService = statisticsApp.NewTesteeStatisticsService(repo, cache)
	module.PlanStatisticsService = statisticsApp.NewPlanStatisticsService(repo, repo, cache, normalized.HotsetRecorder)
	module.ReadService = statisticsApp.NewReadService(
		statisticsReadModelInfra.NewReadModel(normalized.MySQLDB),
		normalized.AnswerSheetReader,
		statisticsApp.WithReadServiceCache(cache),
		statisticsApp.WithReadServiceHotset(normalized.HotsetRecorder),
	)
	module.PeriodicStatsService = statisticsApp.NewPeriodicStatsService(repo)
	module.BehaviorProjectorService = statisticsApp.NewAssessmentEpisodeProjectorWithTransactionRunner(txRunner, repo)
	module.SyncService = statisticsApp.NewSyncServiceWithTransactionRunner(txRunner, repo, normalized.RepairWindowDays, normalized.LockManager)

	return module, nil
}

func normalizeStatisticsModuleDeps(deps StatisticsModuleDeps) (StatisticsModuleDeps, error) {
	if deps.MySQLDB == nil {
		return StatisticsModuleDeps{}, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}
	return deps, nil
}

// Cleanup 清理模块资源
func (m *StatisticsModule) Cleanup() error {
	return nil
}

// CheckHealth 检查模块健康状态
func (m *StatisticsModule) CheckHealth() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *StatisticsModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "statistics",
		Version:     "1.0.0",
		Description: "统计模块",
	}
}
