package assembler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
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
	ScreeningStatisticsService     statisticsApp.ScreeningStatisticsService
	SyncService                    statisticsApp.StatisticsSyncService
	ValidatorService               statisticsApp.StatisticsValidatorService
}

// NewStatisticsModule 创建统计模块
func NewStatisticsModule() *StatisticsModule {
	return &StatisticsModule{}
}

// Initialize 初始化统计模块
// params[0]: *gorm.DB
// params[1]: redis.UniversalClient (Redis缓存客户端)
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

	// 初始化 repository 层
	m.Repo = statisticsInfra.NewStatisticsRepository(mysqlDB)

	// 初始化 cache 层
	if redisClient != nil {
		m.Cache = statisticsCache.NewStatisticsCache(redisClient)
	} else {
		// Redis不可用时，创建空实现（查询时会降级到MySQL）
		m.Cache = nil
	}

	// 初始化 service 层
	m.SystemStatisticsService = statisticsApp.NewSystemStatisticsService(mysqlDB, m.Repo, m.Cache)
	m.QuestionnaireStatisticsService = statisticsApp.NewQuestionnaireStatisticsService(mysqlDB, m.Repo, m.Cache)
	m.TesteeStatisticsService = statisticsApp.NewTesteeStatisticsService(mysqlDB, m.Repo, m.Cache)
	m.PlanStatisticsService = statisticsApp.NewPlanStatisticsService(mysqlDB, m.Repo, m.Cache)

	// TODO: 实现筛查统计服务
	// m.ScreeningStatisticsService = statisticsApp.NewScreeningStatisticsService(mysqlDB, m.Repo, m.Cache)

	// 初始化同步和校验服务
	if m.Cache != nil {
		m.SyncService = statisticsApp.NewSyncService(m.Repo, m.Cache, mysqlDB)
		m.ValidatorService = statisticsApp.NewValidatorService(m.Repo, m.Cache)
	} else {
		// Redis不可用时，同步服务无法工作
		m.SyncService = nil
		m.ValidatorService = nil
	}

	// 初始化 handler 层
	m.Handler = handler.NewStatisticsHandler(
		m.SystemStatisticsService,
		m.QuestionnaireStatisticsService,
		m.TesteeStatisticsService,
		m.PlanStatisticsService,
		m.ScreeningStatisticsService,
		m.SyncService,
		m.ValidatorService,
	)

	return nil
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
