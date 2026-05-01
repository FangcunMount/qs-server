package assembler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsReadModelInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics/readmodel"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
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
	// repository 层
	Repo *statisticsInfra.StatisticsRepository

	// cache 层
	Cache *statisticsCache.StatisticsCache

	// handler 层
	Handler *handler.StatisticsHandler

	// service 层
	SystemStatisticsService        statisticsApp.SystemStatisticsService
	QuestionnaireStatisticsService statisticsApp.QuestionnaireStatisticsService
	TesteeStatisticsService        statisticsApp.TesteeStatisticsService
	PlanStatisticsService          statisticsApp.PlanStatisticsService
	ReadService                    statisticsApp.ReadService
	PeriodicStatsService           statisticsApp.PeriodicStatsService
	SyncService                    statisticsApp.StatisticsSyncService
	BehaviorProjectorService       statisticsApp.BehaviorProjectorService
	testeeAccessService            actorAccessApp.TesteeAccessService
	warmupCoordinator              cachegov.Coordinator
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
	TesteeAccess      actorAccessApp.TesteeAccessService
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
	module.testeeAccessService = normalized.TesteeAccess
	module.warmupCoordinator = normalized.WarmupCoordinator

	// 初始化 repository 层
	module.Repo = statisticsInfra.NewStatisticsRepository(normalized.MySQLDB, mysql.BaseRepositoryOptions{
		Limiter: normalized.MySQLLimiter,
	})

	// 初始化 cache 层
	if normalized.RedisClient != nil {
		module.Cache = statisticsCache.NewStatisticsCacheWithBuilderPolicyVersionStoreAndObserver(
			normalized.RedisClient,
			normalized.CacheBuilder,
			normalized.QueryPolicy,
			normalized.VersionStore,
			normalized.Observer,
		)
	} else {
		// Redis不可用时，创建空实现（查询时会降级到MySQL）
		module.Cache = nil
	}
	txRunner := newMySQLTransactionRunner(normalized.MySQLDB)

	// 初始化 service 层
	module.SystemStatisticsService = statisticsApp.NewSystemStatisticsService(module.Repo, module.Repo, module.Cache, normalized.HotsetRecorder)
	module.QuestionnaireStatisticsService = statisticsApp.NewQuestionnaireStatisticsService(module.Repo, module.Repo, module.Cache, normalized.HotsetRecorder)
	module.TesteeStatisticsService = statisticsApp.NewTesteeStatisticsService(module.Repo, module.Cache)
	module.PlanStatisticsService = statisticsApp.NewPlanStatisticsService(module.Repo, module.Repo, module.Cache, normalized.HotsetRecorder)
	module.ReadService = statisticsApp.NewReadService(
		statisticsReadModelInfra.NewReadModel(normalized.MySQLDB),
		normalized.AnswerSheetReader,
		statisticsApp.WithReadServiceCache(module.Cache),
		statisticsApp.WithReadServiceHotset(normalized.HotsetRecorder),
	)
	module.PeriodicStatsService = statisticsApp.NewPeriodicStatsService(module.Repo)
	module.BehaviorProjectorService = statisticsApp.NewAssessmentEpisodeProjectorWithTransactionRunner(txRunner, module.Repo)
	module.SyncService = statisticsApp.NewSyncServiceWithTransactionRunner(txRunner, module.Repo, normalized.RepairWindowDays, normalized.LockManager)

	// 初始化 handler 层
	module.Handler = handler.NewStatisticsHandler(
		module.SystemStatisticsService,
		module.QuestionnaireStatisticsService,
		module.TesteeStatisticsService,
		module.PlanStatisticsService,
		module.ReadService,
		module.PeriodicStatsService,
		module.SyncService,
	)
	if module.testeeAccessService != nil {
		module.Handler.SetTesteeAccessService(module.testeeAccessService)
	}
	if module.warmupCoordinator != nil {
		module.Handler.SetWarmupCoordinator(module.warmupCoordinator)
	}
	if normalized.StatusService != nil {
		module.Handler.SetCacheGovernanceStatusService(normalized.StatusService)
	}

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
