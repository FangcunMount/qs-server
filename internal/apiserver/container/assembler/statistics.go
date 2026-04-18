package assembler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	surveyAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
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

// NewStatisticsModule 创建统计模块
func NewStatisticsModule() *StatisticsModule {
	return &StatisticsModule{}
}

// Initialize 初始化统计模块
// params[0]: *gorm.DB
// params[1]: redis.UniversalClient (Redis缓存客户端)
// params[2]: string cache namespace（可选）
// params[3]: answersheet.Repository (问卷答卷仓储，可选)
// params[4]: int repair window days（统计批处理默认回补窗口，可选）
// params[5]: cache.CachePolicy 查询缓存策略（可选）
// params[6]: cache.HotsetRecorder（可选）
func (m *StatisticsModule) Initialize(params ...interface{}) error {
	if len(params) < 1 {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is required")
	}

	mysqlDB, ok := params[0].(*gorm.DB)
	if !ok || mysqlDB == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 获取Redis客户端（可选参数）
	var redisClient redis.UniversalClient
	if len(params) > 1 {
		if rc, ok := params[1].(redis.UniversalClient); ok && rc != nil {
			redisClient = rc
		}
	}
	cacheNamespace := ""
	if len(params) > 2 {
		if ns, ok := params[2].(string); ok {
			cacheNamespace = ns
		}
	}
	var answerSheetRepo surveyAnswerSheet.Repository
	if len(params) > 3 {
		if repo, ok := params[3].(surveyAnswerSheet.Repository); ok && repo != nil {
			answerSheetRepo = repo
		}
	}
	repairWindowDays := 0
	if len(params) > 4 {
		if value, ok := params[4].(int); ok {
			repairWindowDays = value
		}
	}
	queryPolicy := cachepolicy.CachePolicy{}
	if len(params) > 5 {
		if value, ok := params[5].(cachepolicy.CachePolicy); ok {
			queryPolicy = value
		}
	}
	var hotset cachepolicy.HotsetRecorder
	if len(params) > 6 {
		if value, ok := params[6].(cachepolicy.HotsetRecorder); ok {
			hotset = value
		}
	}

	// 初始化 repository 层
	m.Repo = statisticsInfra.NewStatisticsRepository(mysqlDB)

	// 初始化 cache 层
	if redisClient != nil {
		m.Cache = statisticsCache.NewStatisticsCacheWithBuilderAndPolicy(redisClient, rediskey.NewBuilderWithNamespace(cacheNamespace), queryPolicy)
	} else {
		// Redis不可用时，创建空实现（查询时会降级到MySQL）
		m.Cache = nil
	}

	// 初始化 service 层
	m.SystemStatisticsService = statisticsApp.NewSystemStatisticsService(mysqlDB, m.Repo, m.Cache, hotset)
	m.QuestionnaireStatisticsService = statisticsApp.NewQuestionnaireStatisticsService(mysqlDB, m.Repo, m.Cache, hotset)
	m.TesteeStatisticsService = statisticsApp.NewTesteeStatisticsService(mysqlDB, m.Repo, m.Cache)
	m.PlanStatisticsService = statisticsApp.NewPlanStatisticsService(mysqlDB, m.Repo, m.Cache, hotset)
	m.ReadService = statisticsApp.NewReadService(mysqlDB, answerSheetRepo)
	m.PeriodicStatsService = statisticsApp.NewPeriodicStatsService(mysqlDB)
	m.BehaviorProjectorService = statisticsApp.NewAssessmentEpisodeProjector(mysqlDB, m.Repo)
	m.SyncService = statisticsApp.NewSyncService(mysqlDB, repairWindowDays)

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
