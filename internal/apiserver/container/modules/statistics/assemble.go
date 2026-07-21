package statistics

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	statisticsV2App "github.com/FangcunMount/qs-server/internal/apiserver/application/statisticsv2"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/cache/statistics"
	statisticsV2Cache "github.com/FangcunMount/qs-server/internal/apiserver/cache/statisticsv2"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	statisticsV2Domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics/v2"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsReadModelInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics/readmodel"
	statisticsV2Infra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statisticsv2"
	statisticsQueryInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	querycache "github.com/FangcunMount/qs-server/internal/pkg/cache/query"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// Module assembles statistics application services.
type Module struct {
	ReadService                statisticsApp.ReadService
	PeriodicStatsService       statisticsApp.PeriodicStatsService
	SyncService                statisticsApp.StatisticsSyncService
	BehaviorProjectorService   statisticsApp.BehaviorProjectorService
	BehaviorJourneyScanService statisticsApp.BehaviorJourneyScanService
	V2Coordinator              *statisticsV2App.Coordinator
	V2RunStore                 *statisticsV2Infra.RunStore
	V2ReadService              *statisticsV2App.ReadService
}

// Deps defines explicit constructor dependencies for the statistics module.
type Deps struct {
	MySQLDB               *gorm.DB
	RedisClient           redis.UniversalClient
	CacheBuilder          *keyspace.Builder
	AnswerSheetScanSource statisticsApp.AnswerSheetScanSource
	MongoDB               *mongo.Database
	RepairWindowDays      int
	CachePolicies         sharedcache.PolicyProvider
	OverviewGuardOpts     statisticsApp.StatisticsReadGuardOptions
	HotsetRecorder        cachetarget.HotsetRecorder
	LockManager           locklease.Manager
	LockRunner            locklease.Runner
	VersionStore          querycache.VersionTokenStore
	Observer              *observability.ComponentObserver
	MySQLLimiter          backpressure.Acquirer
	WarmupCoordinator     statisticsApp.WarmupCoordinator
	StatusService         statisticsApp.GovernanceStatusReader
}

// New assembles the statistics module.
func New(deps Deps) (*Module, error) {
	normalized, err := normalizeDeps(deps)
	if err != nil {
		return nil, err
	}
	module := &Module{}

	repo := statisticsInfra.NewStatisticsRepository(normalized.MySQLDB, mysql.BaseRepositoryOptions{
		Limiter: normalized.MySQLLimiter,
	})

	var cache *statisticsCache.StatisticsCache
	if normalized.RedisClient != nil {
		cache = statisticsCache.NewStatisticsCacheWithBuilderProviderVersionStoreAndObserver(
			normalized.RedisClient,
			normalized.CacheBuilder,
			normalized.CachePolicies,
			normalized.VersionStore,
			normalized.Observer,
		)
	}
	txRunner := modtx.NewMySQLRunner(normalized.MySQLDB)

	readModel := statisticsReadModelInfra.NewReadModel(normalized.MySQLDB, mysql.BaseRepositoryOptions{
		Limiter: normalized.MySQLLimiter,
	})
	module.ReadService = statisticsApp.NewReadService(
		statisticsApp.ReadServiceDeps{
			Overview:   readModel,
			Clinicians: readModel,
			Entries:    readModel,
			Contents:   readModel,
		},
		statisticsApp.WithReadServiceCache(cache),
		statisticsApp.WithReadServiceHotset(normalized.HotsetRecorder),
		statisticsApp.WithReadServiceOverviewGuard(normalized.OverviewGuardOpts),
	)
	module.PeriodicStatsService = statisticsApp.NewPeriodicStatsService(repo)
	module.BehaviorProjectorService = statisticsApp.NewAssessmentEpisodeProjectorWithTransactionRunner(txRunner, repo)
	module.BehaviorJourneyScanService = statisticsApp.NewBehaviorJourneyScanService(
		txRunner,
		repo,
		repo,
		repo,
		normalized.AnswerSheetScanSource,
		statisticsQueryInfra.NewReportScanSource(normalized.MySQLDB, normalized.MongoDB),
	)
	module.SyncService = statisticsApp.NewSyncServiceWithTransactionRunner(txRunner, repo, normalized.RepairWindowDays, normalized.LockManager)
	collectors, err := statisticsV2Domain.NewCollectorSet(
		statisticsV2Infra.NewAccessFactCollector(normalized.MySQLDB),
		statisticsV2Infra.NewAssessmentFactCollector(normalized.MySQLDB, normalized.MongoDB),
		statisticsV2Infra.NewPlanFactCollector(normalized.MySQLDB),
	)
	if err != nil {
		return nil, err
	}
	engine, err := statisticsV2Domain.NewProjectionEngine(statisticsV2Infra.NewProjections(normalized.MySQLDB)...)
	if err != nil {
		return nil, err
	}
	module.V2RunStore = statisticsV2Infra.NewRunStore(normalized.MySQLDB)
	module.V2ReadService = statisticsV2App.NewReadService(
		statisticsV2Infra.NewReadStore(normalized.MySQLDB, normalized.MySQLLimiter),
		statisticsV2Cache.NewQueryCache(normalized.RedisClient),
	)
	module.V2Coordinator = statisticsV2App.NewCoordinator(
		collectors,
		engine,
		module.V2RunStore,
		txRunner,
		normalized.LockRunner,
		statisticsV2Cache.NewPublisher(
			statisticsV2Cache.NewGenerationPublisher(normalized.RedisClient),
			module.V2ReadService,
		),
	)

	return module, nil
}

func normalizeDeps(deps Deps) (Deps, error) {
	if deps.MySQLDB == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}
	if deps.MongoDB == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "mongo database connection is nil")
	}
	if deps.LockRunner == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "lock runner is nil")
	}
	return deps, nil
}

// Cleanup releases module resources.
func (m *Module) Cleanup() error {
	return nil
}

// CheckHealth verifies module health.
func (m *Module) CheckHealth() error {
	return nil
}

// ModuleInfo returns module metadata.
func (m *Module) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:        string(Name),
		Version:     "1.0.0",
		Description: "统计模块",
	}
}
