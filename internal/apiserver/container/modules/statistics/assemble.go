package statistics

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/cache/statistics"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsReadModelInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics/readmodel"
	statisticsQueryInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	querycache "github.com/FangcunMount/qs-server/internal/pkg/cache/query"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// Module assembles statistics application services.
type Module struct {
	SystemStatisticsService        statisticsApp.SystemStatisticsService
	QuestionnaireStatisticsService statisticsApp.QuestionnaireStatisticsService
	TesteeStatisticsService        statisticsApp.TesteeStatisticsService
	PlanStatisticsService          statisticsApp.PlanStatisticsService
	ReadService                    statisticsApp.ReadService
	PeriodicStatsService           statisticsApp.PeriodicStatsService
	SyncService                    statisticsApp.StatisticsSyncService
	BehaviorProjectorService       statisticsApp.BehaviorProjectorService
	BehaviorJourneyScanService     statisticsApp.BehaviorJourneyScanService
}

// Deps defines explicit constructor dependencies for the statistics module.
type Deps struct {
	MySQLDB                *gorm.DB
	RedisClient            redis.UniversalClient
	CacheBuilder           *keyspace.Builder
	AnswerSheetReader      surveyreadmodel.AnswerSheetReader
	AnswerSheetScanSource  statisticsApp.AnswerSheetScanSource
	MongoDB                *mongo.Database
	RepairWindowDays       int
	QueryPolicy            cachepolicy.CachePolicy
	SystemStatisticsOpts   statisticsApp.SystemStatisticsOptions
	OverviewGuardOpts      statisticsApp.StatisticsReadGuardOptions
	QuestionnaireGuardOpts statisticsApp.StatisticsReadGuardOptions
	HotsetRecorder         cachetarget.HotsetRecorder
	LockManager            locklease.Manager
	VersionStore           querycache.VersionTokenStore
	Observer               *observability.ComponentObserver
	MySQLLimiter           backpressure.Acquirer
	WarmupCoordinator      cachegov.Coordinator
	StatusService          cachegov.StatusService
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
		cache = statisticsCache.NewStatisticsCacheWithBuilderPolicyVersionStoreAndObserver(
			normalized.RedisClient,
			normalized.CacheBuilder,
			normalized.QueryPolicy,
			normalized.VersionStore,
			normalized.Observer,
		)
	}
	txRunner := modtx.NewMySQLRunner(normalized.MySQLDB)

	module.SystemStatisticsService = statisticsApp.NewSystemStatisticsService(
		repo,
		repo,
		cache,
		normalized.HotsetRecorder,
		statisticsApp.WithSystemStatisticsOptions(normalized.SystemStatisticsOpts),
	)
	module.QuestionnaireStatisticsService = statisticsApp.NewQuestionnaireStatisticsService(
		repo, repo, cache, normalized.HotsetRecorder,
		statisticsApp.WithQuestionnaireStatisticsGuard(normalized.QuestionnaireGuardOpts),
	)
	module.TesteeStatisticsService = statisticsApp.NewTesteeStatisticsService(repo, cache)
	module.PlanStatisticsService = statisticsApp.NewPlanStatisticsService(repo, repo, cache, normalized.HotsetRecorder)
	module.ReadService = statisticsApp.NewReadService(
		statisticsReadModelInfra.NewReadModel(normalized.MySQLDB),
		normalized.AnswerSheetReader,
		statisticsApp.WithReadServiceCache(cache),
		statisticsApp.WithReadServiceHotset(normalized.HotsetRecorder),
		statisticsApp.WithReadServiceOverviewGuard(normalized.OverviewGuardOpts),
	)
	module.PeriodicStatsService = statisticsApp.NewPeriodicStatsService(repo)
	module.BehaviorProjectorService = statisticsApp.NewAssessmentEpisodeProjectorWithTransactionRunner(txRunner, repo)
	module.BehaviorJourneyScanService = statisticsApp.NewBehaviorJourneyScanService(
		txRunner,
		repo,
		normalized.AnswerSheetScanSource,
		statisticsQueryInfra.NewReportScanSource(normalized.MySQLDB, normalized.MongoDB),
	)
	module.SyncService = statisticsApp.NewSyncServiceWithTransactionRunner(txRunner, repo, normalized.RepairWindowDays, normalized.LockManager)

	return module, nil
}

func normalizeDeps(deps Deps) (Deps, error) {
	if deps.MySQLDB == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
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
