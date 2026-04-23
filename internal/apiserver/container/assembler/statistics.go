package assembler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	surveyAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsReadModelInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics/readmodel"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
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

type statisticsModuleDeps struct {
	mysqlDB          *gorm.DB
	redisClient      redis.UniversalClient
	cacheBuilder     *rediskey.Builder
	answerSheetRepo  surveyAnswerSheet.Repository
	repairWindowDays int
	queryPolicy      cachepolicy.CachePolicy
	hotsetRecorder   scaleCache.HotsetRecorder
	lockManager      *redislock.Manager
	versionStore     scaleCache.VersionTokenStore
	observer         *scaleCache.Observer
}

// NewStatisticsModule 创建统计模块
func NewStatisticsModule() *StatisticsModule {
	return &StatisticsModule{}
}

// Initialize 初始化统计模块
// params[0]: *gorm.DB
// params[1]: redis.UniversalClient (Redis缓存客户端)
// params[2]: *rediskey.Builder（可选）
// params[3]: answersheet.Repository (问卷答卷仓储，可选)
// params[4]: int repair window days（统计批处理默认回补窗口，可选）
// params[5]: cachepolicy.CachePolicy 查询缓存策略（可选）
// params[6]: scaleCache.HotsetRecorder（可选）
// params[7]: *redislock.Manager（统计同步锁，可选）
func (m *StatisticsModule) Initialize(params ...interface{}) error {
	deps, err := parseStatisticsModuleDeps(params)
	if err != nil {
		return err
	}

	// 初始化 repository 层
	m.Repo = statisticsInfra.NewStatisticsRepository(deps.mysqlDB)

	// 初始化 cache 层
	if deps.redisClient != nil {
		m.Cache = statisticsCache.NewStatisticsCacheWithBuilderPolicyVersionStoreAndObserver(
			deps.redisClient,
			deps.cacheBuilder,
			deps.queryPolicy,
			deps.versionStore,
			deps.observer,
		)
	} else {
		// Redis不可用时，创建空实现（查询时会降级到MySQL）
		m.Cache = nil
	}

	// 初始化 service 层
	m.SystemStatisticsService = statisticsApp.NewSystemStatisticsService(deps.mysqlDB, m.Repo, m.Cache, deps.hotsetRecorder)
	m.QuestionnaireStatisticsService = statisticsApp.NewQuestionnaireStatisticsService(deps.mysqlDB, m.Repo, m.Cache, deps.hotsetRecorder)
	m.TesteeStatisticsService = statisticsApp.NewTesteeStatisticsService(deps.mysqlDB, m.Repo, m.Cache)
	m.PlanStatisticsService = statisticsApp.NewPlanStatisticsService(deps.mysqlDB, m.Repo, m.Cache, deps.hotsetRecorder)
	m.ReadService = statisticsApp.NewReadService(statisticsReadModelInfra.NewReadModel(deps.mysqlDB), deps.answerSheetRepo)
	m.PeriodicStatsService = statisticsApp.NewPeriodicStatsService(deps.mysqlDB)
	m.BehaviorProjectorService = statisticsApp.NewAssessmentEpisodeProjector(deps.mysqlDB, m.Repo)
	m.SyncService = statisticsApp.NewSyncService(deps.mysqlDB, deps.repairWindowDays, deps.lockManager)

	// 初始化 handler 层
	m.Handler = handler.NewStatisticsHandler(
		m.SystemStatisticsService,
		m.QuestionnaireStatisticsService,
		m.TesteeStatisticsService,
		m.PlanStatisticsService,
		m.ReadService,
		m.PeriodicStatsService,
		m.SyncService,
	)
	if m.testeeAccessService != nil {
		m.Handler.SetTesteeAccessService(m.testeeAccessService)
	}
	if m.warmupCoordinator != nil {
		m.Handler.SetWarmupCoordinator(m.warmupCoordinator)
	}

	return nil
}

func parseStatisticsModuleDeps(params []interface{}) (*statisticsModuleDeps, error) {
	if len(params) < 1 {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is required")
	}

	mysqlDB, ok := params[0].(*gorm.DB)
	if !ok || mysqlDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	deps := &statisticsModuleDeps{mysqlDB: mysqlDB}
	applyOptionalParam(params, 1, func(client redis.UniversalClient) {
		if client != nil {
			deps.redisClient = client
		}
	})
	applyOptionalParam(params, 2, func(builder *rediskey.Builder) {
		deps.cacheBuilder = builder
	})
	applyOptionalParam(params, 3, func(repo surveyAnswerSheet.Repository) {
		if repo != nil {
			deps.answerSheetRepo = repo
		}
	})
	applyOptionalParam(params, 4, func(value int) {
		deps.repairWindowDays = value
	})
	applyOptionalParam(params, 5, func(value cachepolicy.CachePolicy) {
		deps.queryPolicy = value
	})
	applyOptionalParam(params, 6, func(value scaleCache.HotsetRecorder) {
		deps.hotsetRecorder = value
	})
	applyOptionalParam(params, 7, func(value *redislock.Manager) {
		deps.lockManager = value
	})
	applyOptionalParam(params, 8, func(value scaleCache.VersionTokenStore) {
		deps.versionStore = value
	})
	applyOptionalParam(params, 9, func(value *scaleCache.Observer) {
		deps.observer = value
	})
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
