package statistics

import (
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/cache/statistics"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

type Module struct {
	Coordinator *statisticsApp.Coordinator
	RunStore    *statisticsInfra.RunStore
	ReadService *statisticsApp.ReadService
}

type Deps struct {
	MySQLDB      *gorm.DB
	MongoDB      *mongo.Database
	RedisClient  redis.UniversalClient
	LockRunner   locklease.Runner
	MySQLLimiter backpressure.Acquirer
	QueryTTL     time.Duration
}

func New(deps Deps) (*Module, error) {
	if deps.MySQLDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}
	if deps.MongoDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "mongo database connection is nil")
	}
	if deps.LockRunner == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "lock runner is nil")
	}

	collectors, err := statisticsDomain.NewCollectorSet(
		statisticsInfra.NewAccessFactCollector(deps.MySQLDB),
		statisticsInfra.NewAssessmentFactCollector(deps.MySQLDB, deps.MongoDB),
		statisticsInfra.NewPlanFactCollector(deps.MySQLDB),
	)
	if err != nil {
		return nil, err
	}
	dailyEngine, err := statisticsDomain.NewProjectionEngine(statisticsInfra.NewDailyProjections(deps.MySQLDB)...)
	if err != nil {
		return nil, err
	}
	globalEngine, err := statisticsDomain.NewProjectionEngine(statisticsInfra.NewGlobalProjections(deps.MySQLDB)...)
	if err != nil {
		return nil, err
	}

	module := &Module{RunStore: statisticsInfra.NewRunStore(deps.MySQLDB)}
	module.ReadService = statisticsApp.NewReadService(
		statisticsInfra.NewReadStore(deps.MySQLDB, deps.MySQLLimiter),
		statisticsCache.NewQueryCache(deps.RedisClient, deps.QueryTTL),
	)
	module.Coordinator = statisticsApp.NewCoordinator(
		collectors,
		dailyEngine,
		globalEngine,
		module.RunStore,
		modtx.NewMySQLRunner(deps.MySQLDB),
		deps.LockRunner,
		statisticsCache.NewPublisher(
			statisticsCache.NewGenerationPublisher(deps.RedisClient),
			module.ReadService,
		),
	)
	return module, nil
}

func (m *Module) Cleanup() error     { return nil }
func (m *Module) CheckHealth() error { return nil }
func (m *Module) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{Name: string(Name), Version: "2.0.0", Description: "统计事实、投影与查询模块"}
}
