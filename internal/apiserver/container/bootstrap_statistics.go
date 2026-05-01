package container

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

func (c *Container) buildStatisticsModuleDeps() assembler.StatisticsModuleDeps {
	versionStore := cachequery.NewStaticVersionTokenStore(0)
	if c == nil {
		return assembler.StatisticsModuleDeps{
			VersionStore: versionStore,
		}
	}

	disableStatisticsCache := c.cacheOptions.DisableStatisticsCache
	redisClient := c.redisCache
	if !disableStatisticsCache {
		redisClient = c.CacheClient(cacheplane.FamilyQuery)
	}
	if disableStatisticsCache {
		redisClient = nil
	}

	var answerSheetReader surveyreadmodel.AnswerSheetReader
	if c.surveyScaleInfra != nil {
		answerSheetReader = c.surveyScaleInfra.answerSheetReader
	}
	if !disableStatisticsCache {
		versionStore = cachequery.NewRedisVersionTokenStoreWithKindAndObserver(
			c.CacheClient(cacheplane.FamilyMeta),
			string(cachepolicy.PolicyStatsQuery),
			c.cacheObserver(),
		)
		if versionStore == nil {
			versionStore = cachequery.NewStaticVersionTokenStore(0)
		}
	}

	return assembler.StatisticsModuleDeps{
		MySQLDB:           c.mysqlDB,
		RedisClient:       redisClient,
		CacheBuilder:      c.CacheBuilder(cacheplane.FamilyQuery),
		AnswerSheetReader: answerSheetReader,
		RepairWindowDays:  c.statisticsRepairWindowDays,
		QueryPolicy:       c.CachePolicy(cachepolicy.PolicyStatsQuery),
		HotsetRecorder:    c.hotsetRecorder(),
		LockManager:       c.CacheLockManager(),
		VersionStore:      versionStore,
		Observer:          c.cacheObserver(),
		MySQLLimiter:      c.backpressure.MySQL,
		TesteeAccess:      c.actorTesteeAccessService(),
		WarmupCoordinator: c.WarmupCoordinator(),
		StatusService:     c.CacheGovernanceStatusService(),
	}
}

func (c *Container) buildStatisticsModule() (*assembler.StatisticsModule, error) {
	return assembler.NewStatisticsModule(c.buildStatisticsModuleDeps())
}

// initStatisticsModule 初始化 Statistics 模块。
func (c *Container) initStatisticsModule() error {
	statisticsModule, err := c.buildStatisticsModule()
	if err != nil {
		return fmt.Errorf("failed to initialize statistics module: %w", err)
	}

	c.StatisticsModule = statisticsModule
	c.registerModule("statistics", statisticsModule)

	c.printf("📦 Statistics module initialized\n")
	return nil
}

func (c *Container) initWarmupCoordinator() error {
	if c == nil {
		return nil
	}
	if c.cache != nil {
		c.cache.BindGovernance(newCacheGovernanceAdapter(c).bindings())
	}
	return nil
}
