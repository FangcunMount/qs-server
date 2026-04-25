package assembler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	surveyAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsReadModelInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics/readmodel"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
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
	MySQLDB          *gorm.DB
	RedisClient      redis.UniversalClient
	CacheBuilder     *rediskey.Builder
	AnswerSheetRepo  surveyAnswerSheet.Repository
	RepairWindowDays int
	QueryPolicy      cachepolicy.CachePolicy
	HotsetRecorder   cachetarget.HotsetRecorder
	LockManager      *redislock.Manager
	VersionStore     cachequery.VersionTokenStore
	Observer         *cacheobservability.ComponentObserver
	MySQLLimiter     backpressure.Acquirer
}

// NewStatisticsModule 创建统计模块。
func NewStatisticsModule(deps StatisticsModuleDeps) (*StatisticsModule, error) {
	normalized, err := normalizeStatisticsModuleDeps(deps)
	if err != nil {
		return nil, err
	}
	module := &StatisticsModule{}

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

	// 初始化 service 层
	module.SystemStatisticsService = statisticsApp.NewSystemStatisticsService(normalized.MySQLDB, module.Repo, module.Cache, normalized.HotsetRecorder)
	module.QuestionnaireStatisticsService = statisticsApp.NewQuestionnaireStatisticsService(normalized.MySQLDB, module.Repo, module.Cache, normalized.HotsetRecorder)
	module.TesteeStatisticsService = statisticsApp.NewTesteeStatisticsService(normalized.MySQLDB, module.Repo, module.Cache)
	module.PlanStatisticsService = statisticsApp.NewPlanStatisticsService(normalized.MySQLDB, module.Repo, module.Cache, normalized.HotsetRecorder)
	module.ReadService = statisticsApp.NewReadService(statisticsReadModelInfra.NewReadModel(normalized.MySQLDB), normalized.AnswerSheetRepo)
	module.PeriodicStatsService = statisticsApp.NewPeriodicStatsService(normalized.MySQLDB)
	module.BehaviorProjectorService = statisticsApp.NewAssessmentEpisodeProjector(normalized.MySQLDB, module.Repo)
	module.SyncService = statisticsApp.NewSyncService(normalized.MySQLDB, normalized.RepairWindowDays, normalized.LockManager)

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

	return module, nil
}

func normalizeStatisticsModuleDeps(deps StatisticsModuleDeps) (StatisticsModuleDeps, error) {
	if deps.MySQLDB == nil {
		return StatisticsModuleDeps{}, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}
	return deps, nil
}

// SetTesteeAccessService 设置 testee 访问控制服务。
func (m *StatisticsModule) SetTesteeAccessService(testeeAccessService actorAccessApp.TesteeAccessService) {
	m.testeeAccessService = testeeAccessService
	if m.Handler != nil {
		m.Handler.SetTesteeAccessService(testeeAccessService)
	}
}

func (m *StatisticsModule) SetWarmupCoordinator(coordinator cachegov.Coordinator) {
	m.warmupCoordinator = coordinator
	if m.Handler != nil {
		m.Handler.SetWarmupCoordinator(coordinator)
	}
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
